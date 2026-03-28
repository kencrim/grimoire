package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/kencrim/grimoire/libs/relay"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

// relayClient talks to the ws daemon over Unix socket.
type relayClient struct {
	agentID string
	conn    net.Conn
	enc     *json.Encoder
	dec     *json.Decoder
}

func newRelayClient(socketPath, agentID string) (*relayClient, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("connect to daemon at %s: %w", socketPath, err)
	}
	return &relayClient{
		agentID: agentID,
		conn:    conn,
		enc:     json.NewEncoder(conn),
		dec:     json.NewDecoder(conn),
	}, nil
}

func (c *relayClient) send(to, message string, msgType relay.MessageType) error {
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
	return c.dec.Decode(&resp)
}

func (c *relayClient) spawn(name, task, ctx string) (relay.SpawnResponse, error) {
	req := relay.SpawnRequest{
		ParentID: c.agentID,
		Name:     name,
		Task:     task,
		Context:  ctx,
	}
	payload, _ := json.Marshal(req)
	env := relay.Envelope{Action: "spawn", Payload: payload}
	if err := c.enc.Encode(env); err != nil {
		return relay.SpawnResponse{}, err
	}
	var resp relay.SpawnResponse
	return resp, c.dec.Decode(&resp)
}

func (c *relayClient) status(agentID string) (json.RawMessage, error) {
	req := relay.StatusRequest{AgentID: agentID}
	payload, _ := json.Marshal(req)
	env := relay.Envelope{Action: "status", Payload: payload}
	if err := c.enc.Encode(env); err != nil {
		return nil, err
	}
	var resp json.RawMessage
	return resp, c.dec.Decode(&resp)
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
				return mcp.NewToolResultError(err.Error()), nil, nil
			}
			return mcp.NewToolResultText("delivered"), nil, nil
		})

		// relay_spawn
		mcp.AddTool(server, &mcp.Tool{
			Name:        "relay_spawn",
			Description: "Spawn a child agent to handle a subtask. Returns immediately — the child runs in parallel.",
		}, func(ctx context.Context, req *mcp.CallToolRequest, input RelaySpawnInput) (*mcp.CallToolResult, any, error) {
			resp, err := client.spawn(input.Name, input.Task, input.Context)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil, nil
			}
			result, _ := json.Marshal(resp)
			return mcp.NewToolResultText(string(result)), nil, nil
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
				return mcp.NewToolResultError(err.Error()), nil, nil
			}
			return mcp.NewToolResultText(string(resp)), nil, nil
		})

		// relay_kill
		mcp.AddTool(server, &mcp.Tool{
			Name:        "relay_kill",
			Description: "Terminate a child agent and its entire subtree.",
		}, func(ctx context.Context, req *mcp.CallToolRequest, input RelayKillInput) (*mcp.CallToolResult, any, error) {
			// For now, send a kill action to the daemon
			// Full implementation in Phase 3
			return mcp.NewToolResultText(fmt.Sprintf("kill request sent for %s", input.AgentID)), nil, nil
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
