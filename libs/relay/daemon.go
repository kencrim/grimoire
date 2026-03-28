package relay

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
)

// AgentHandle represents a running agent process.
type AgentHandle struct {
	ID           string
	ParentID     string
	Agent        string    // claude, amp, codex
	Stdin        io.Writer // JSONL injection point
	WorktreePath string
	Session      string // tmux session name
	Status       string // alive, idle, exited
}

// Daemon manages the agent registry and routes messages.
type Daemon struct {
	agents   map[string]*AgentHandle
	mu       sync.RWMutex
	listener net.Listener
	onSpawn  func(SpawnRequest) (SpawnResponse, error) // callback for spawning
}

// DefaultSocketPath returns the Unix socket path for the daemon.
func DefaultSocketPath() string {
	return filepath.Join(os.TempDir(), "ws-relay.sock")
}

// NewDaemon creates a new relay daemon.
func NewDaemon() *Daemon {
	return &Daemon{
		agents: make(map[string]*AgentHandle),
	}
}

// SetSpawnHandler sets the callback invoked when an agent requests a spawn.
func (d *Daemon) SetSpawnHandler(fn func(SpawnRequest) (SpawnResponse, error)) {
	d.onSpawn = fn
}

// Register adds an agent to the registry.
func (d *Daemon) Register(handle *AgentHandle) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.agents[handle.ID] = handle
	log.Printf("[daemon] registered agent %q", handle.ID)
}

// Unregister removes an agent from the registry.
func (d *Daemon) Unregister(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.agents, id)
	log.Printf("[daemon] unregistered agent %q", id)
}

// Route delivers a message to the appropriate agent(s).
func (d *Daemon) Route(msg Message) error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	target := msg.To

	switch target {
	case "parent":
		sender, ok := d.agents[msg.From]
		if !ok {
			return fmt.Errorf("sender %q not found", msg.From)
		}
		if sender.ParentID == "" {
			return fmt.Errorf("agent %q has no parent", msg.From)
		}
		return d.deliver(sender.ParentID, msg)

	case "siblings":
		sender, ok := d.agents[msg.From]
		if !ok {
			return fmt.Errorf("sender %q not found", msg.From)
		}
		for _, agent := range d.agents {
			if agent.ParentID == sender.ParentID && agent.ID != msg.From {
				if err := d.deliver(agent.ID, msg); err != nil {
					log.Printf("[daemon] failed to deliver to sibling %q: %v", agent.ID, err)
				}
			}
		}
		return nil

	default:
		return d.deliver(target, msg)
	}
}

// deliver writes a message to an agent's stdin in Amp JSONL format.
func (d *Daemon) deliver(agentID string, msg Message) error {
	agent, ok := d.agents[agentID]
	if !ok {
		return fmt.Errorf("agent %q not found", agentID)
	}
	if agent.Stdin == nil {
		return fmt.Errorf("agent %q has no stdin writer", agentID)
	}

	ampMsg := FormatForAmp(msg)
	data, err := json.Marshal(ampMsg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	data = append(data, '\n')

	if _, err := agent.Stdin.Write(data); err != nil {
		return fmt.Errorf("write to %q stdin: %w", agentID, err)
	}

	log.Printf("[daemon] delivered message from %q to %q", msg.From, agentID)
	return nil
}

// ListAgents returns all registered agents.
func (d *Daemon) ListAgents() []AgentStatus {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var result []AgentStatus
	for _, a := range d.agents {
		result = append(result, AgentStatus{
			ID:     a.ID,
			Status: a.Status,
			Agent:  a.Agent,
		})
	}
	return result
}

// Listen starts the Unix socket listener for MCP adapter connections.
func (d *Daemon) Listen(socketPath string) error {
	os.Remove(socketPath)
	l, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen %s: %w", socketPath, err)
	}
	d.listener = l
	log.Printf("[daemon] listening on %s", socketPath)

	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		go d.handleConn(conn)
	}
}

// Close stops the daemon.
func (d *Daemon) Close() error {
	if d.listener != nil {
		return d.listener.Close()
	}
	return nil
}

// Wire protocol: newline-delimited JSON over Unix socket.
// Each line is an Envelope with a type field that determines the payload.

// Envelope is the wire format between MCP adapters and the daemon.
type Envelope struct {
	Action  string          `json:"action"` // send, spawn, status, register, kill
	Payload json.RawMessage `json:"payload"`
}

// RegisterPayload is sent by an MCP adapter to register its agent.
type RegisterPayload struct {
	AgentID  string `json:"agent_id"`
	ParentID string `json:"parent_id,omitempty"`
	Agent    string `json:"agent"`
}

func (d *Daemon) handleConn(conn net.Conn) {
	defer conn.Close()
	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	for {
		var env Envelope
		if err := decoder.Decode(&env); err != nil {
			if err != io.EOF {
				log.Printf("[daemon] decode error: %v", err)
			}
			return
		}

		var resp any
		var err error

		switch env.Action {
		case "send":
			var msg Message
			if err = json.Unmarshal(env.Payload, &msg); err == nil {
				err = d.Route(msg)
			}
			resp = map[string]string{"status": "delivered"}

		case "spawn":
			var req SpawnRequest
			if err = json.Unmarshal(env.Payload, &req); err == nil {
				if d.onSpawn != nil {
					var sr SpawnResponse
					sr, err = d.onSpawn(req)
					resp = sr
				} else {
					err = fmt.Errorf("no spawn handler configured")
				}
			}

		case "status":
			var req StatusRequest
			if err = json.Unmarshal(env.Payload, &req); err == nil {
				if req.AgentID == "all" || req.AgentID == "" {
					resp = d.ListAgents()
				} else {
					d.mu.RLock()
					if a, ok := d.agents[req.AgentID]; ok {
						resp = AgentStatus{ID: a.ID, Status: a.Status, Agent: a.Agent}
					} else {
						err = fmt.Errorf("agent %q not found", req.AgentID)
					}
					d.mu.RUnlock()
				}
			}

		case "register":
			var reg RegisterPayload
			if err = json.Unmarshal(env.Payload, &reg); err == nil {
				// Registration is noted but stdin handle must be set separately
				log.Printf("[daemon] MCP adapter registered for agent %q", reg.AgentID)
				resp = map[string]string{"status": "registered"}
			}

		default:
			err = fmt.Errorf("unknown action %q", env.Action)
		}

		if err != nil {
			encoder.Encode(map[string]string{"error": err.Error()})
		} else {
			encoder.Encode(resp)
		}
	}
}
