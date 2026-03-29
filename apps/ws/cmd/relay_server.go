package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/kencrim/grimoire/libs/relay"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

func toolText(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

func toolError(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}, IsError: true}
}

// relayClient talks to the ws daemon over Unix socket.
type relayClient struct {
	agentID    string
	socketPath string
	conn       net.Conn
	enc        *json.Encoder
	dec        *json.Decoder
	mu         sync.Mutex
}

func newRelayClient(socketPath, agentID string) (*relayClient, error) {
	c := &relayClient{
		agentID:    agentID,
		socketPath: socketPath,
	}
	if err := c.connect(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *relayClient) connect() error {
	conn, err := net.Dial("unix", c.socketPath)
	if err != nil {
		return fmt.Errorf("connect to daemon at %s: %w", c.socketPath, err)
	}
	c.conn = conn
	c.enc = json.NewEncoder(conn)
	c.dec = json.NewDecoder(conn)
	return nil
}

func (c *relayClient) reconnect() error {
	if c.conn != nil {
		c.conn.Close()
	}
	return c.connect()
}

func (c *relayClient) doWithRetry(fn func() error) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	backoffs := []time.Duration{100 * time.Millisecond, 1 * time.Second, 5 * time.Second}

	// First attempt without reconnect
	err := fn()
	if err == nil || !isConnError(err) {
		return err
	}

	// Retry with reconnect
	for i, backoff := range backoffs {
		log.Printf("[relay-client] connection error, retry %d/%d after %v: %v", i+1, len(backoffs), backoff, err)
		time.Sleep(backoff)
		if reconnErr := c.reconnect(); reconnErr != nil {
			err = reconnErr
			continue
		}
		err = fn()
		if err == nil || !isConnError(err) {
			return err
		}
	}
	return fmt.Errorf("after %d retries: %w", len(backoffs), err)
}

func isConnError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
		return true
	}
	if errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	return false
}

func (c *relayClient) send(to, message string, msgType relay.MessageType) error {
	return c.doWithRetry(func() error {
		msg := relay.Message{
			From:    c.agentID,
			To:      to,
			Type:    msgType,
			Content: message,
			Time:    time.Now(),
		}
		payload, _ := json.Marshal(msg)
		env := relay.Envelope{Action: "send", Payload: payload}
		if err := c.enc.Encode(env); err != nil {
			return err
		}
		var resp map[string]string
		if err := c.dec.Decode(&resp); err != nil {
			return err
		}
		if errMsg, ok := resp["error"]; ok {
			return fmt.Errorf("%s", errMsg)
		}
		return nil
	})
}

func (c *relayClient) spawn(name, task, ctx string) (relay.SpawnResponse, error) {
	var resp relay.SpawnResponse
	err := c.doWithRetry(func() error {
		req := relay.SpawnRequest{
			ParentID: c.agentID,
			Name:     name,
			Task:     task,
			Context:  ctx,
		}
		payload, _ := json.Marshal(req)
		env := relay.Envelope{Action: "spawn", Payload: payload}
		if err := c.enc.Encode(env); err != nil {
			return err
		}
		return c.dec.Decode(&resp)
	})
	return resp, err
}

func (c *relayClient) status(agentID string) (json.RawMessage, error) {
	var resp json.RawMessage
	err := c.doWithRetry(func() error {
		req := relay.StatusRequest{AgentID: agentID}
		payload, _ := json.Marshal(req)
		env := relay.Envelope{Action: "status", Payload: payload}
		if err := c.enc.Encode(env); err != nil {
			return err
		}
		return c.dec.Decode(&resp)
	})
	return resp, err
}

func (c *relayClient) kill(agentID string) (relay.KillResponse, error) {
	var resp relay.KillResponse
	err := c.doWithRetry(func() error {
		req := relay.KillRequest{AgentID: agentID}
		payload, _ := json.Marshal(req)
		env := relay.Envelope{Action: "kill", Payload: payload}
		if err := c.enc.Encode(env); err != nil {
			return err
		}
		return c.dec.Decode(&resp)
	})
	return resp, err
}

func (c *relayClient) close() {
	c.conn.Close()
}

// MCP tool input types

type RelaySendInput struct {
	To      string `json:"to" jsonschema:"Target agent name (e.g. 'auth', 'parent', 'siblings')"`
	Message string `json:"message" jsonschema:"Message content"`
	Type    string `json:"type,omitempty" jsonschema:"Message type: task, result, status, question, error"`
}

type RelaySpawnInput struct {
	Name    string `json:"name" jsonschema:"Short identifier for the child agent"`
	Task    string `json:"task" jsonschema:"Task description for the child agent"`
	Context string `json:"context,omitempty" jsonschema:"Additional context files or instructions"`
}

type RelayStatusInput struct {
	AgentID string `json:"agent_id,omitempty" jsonschema:"Agent to check, or 'all' for full DAG status"`
}

type RelayKillInput struct {
	AgentID string `json:"agent_id" jsonschema:"Agent to terminate"`
}

var relayServerCmd = &cobra.Command{
	Use:    "relay-server",
	Short:  "MCP relay server adapter (launched by Amp, not directly)",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		agentID, _ := cmd.Flags().GetString("agent-id")
		socketPath, _ := cmd.Flags().GetString("socket")

		if agentID == "" || socketPath == "" {
			return fmt.Errorf("--agent-id and --socket are required")
		}

		client, err := newRelayClient(socketPath, agentID)
		if err != nil {
			return err
		}
		defer client.close()

		server := mcp.NewServer(
			&mcp.Implementation{Name: "ws-relay", Version: "0.1.0"},
			nil,
		)

		// relay_send
		mcp.AddTool(server, &mcp.Tool{
			Name:        "relay_send",
			Description: "Send a message to another agent in the workstream DAG. Use to='parent' to report to your supervisor, to='siblings' to broadcast to peers.",
		}, func(ctx context.Context, req *mcp.CallToolRequest, input RelaySendInput) (*mcp.CallToolResult, any, error) {
			msgType := relay.MsgStatus
			if input.Type != "" {
				msgType = relay.MessageType(input.Type)
			}
			if err := client.send(input.To, input.Message, msgType); err != nil {
				return toolError(err.Error()), nil, nil
			}
			return toolText("delivered"), nil, nil
		})

		// relay_spawn
		mcp.AddTool(server, &mcp.Tool{
			Name:        "relay_spawn",
			Description: "Spawn a child agent to handle a subtask. Returns immediately — the child runs in parallel.",
		}, func(ctx context.Context, req *mcp.CallToolRequest, input RelaySpawnInput) (*mcp.CallToolResult, any, error) {
			resp, err := client.spawn(input.Name, input.Task, input.Context)
			if err != nil {
				return toolError(err.Error()), nil, nil
			}
			result, _ := json.Marshal(resp)
			return toolText(string(result)), nil, nil
		})

		// relay_status
		mcp.AddTool(server, &mcp.Tool{
			Name:        "relay_status",
			Description: "Check the status of another agent or the full DAG. Use agent_id='all' for full status.",
		}, func(ctx context.Context, req *mcp.CallToolRequest, input RelayStatusInput) (*mcp.CallToolResult, any, error) {
			if input.AgentID == "" {
				input.AgentID = "all"
			}
			resp, err := client.status(input.AgentID)
			if err != nil {
				return toolError(err.Error()), nil, nil
			}
			return toolText(string(resp)), nil, nil
		})

		// relay_kill
		mcp.AddTool(server, &mcp.Tool{
			Name:        "relay_kill",
			Description: "Terminate a child agent and its entire subtree.",
		}, func(ctx context.Context, req *mcp.CallToolRequest, input RelayKillInput) (*mcp.CallToolResult, any, error) {
			resp, err := client.kill(input.AgentID)
			if err != nil {
				return toolError(err.Error()), nil, nil
			}
			result, _ := json.Marshal(resp)
			return toolText(string(result)), nil, nil
		})

		log.Printf("[relay-server] agent=%s socket=%s — MCP server starting on stdio", agentID, socketPath)
		return server.Run(cmd.Context(), &mcp.StdioTransport{})
	},
}

func init() {
	relayServerCmd.Flags().String("agent-id", "", "This agent's ID in the DAG")
	relayServerCmd.Flags().String("socket", "", "Daemon Unix socket path")
	rootCmd.AddCommand(relayServerCmd)
}
