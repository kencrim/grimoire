package relay

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AgentHandle represents a running agent process.
type AgentHandle struct {
	ID           string
	ParentID     string
	Agent        string    // claude, amp, codex
	Stdin        io.Writer // JSONL injection point
	WorktreePath string
	Session      string // tmux session name
	PaneID       string // tmux pane ID (e.g. %5) — used for targeting split panes
	Host         string // SSH host for remote agents (empty = local)
	Status       string // alive, idle, exited
}

// Daemon manages the agent registry and routes messages.
type Daemon struct {
	agents    map[string]*AgentHandle
	mu        sync.RWMutex
	listener  net.Listener
	onSpawn   func(SpawnRequest) (SpawnResponse, error)   // callback for spawning
	onKill    func(KillRequest) (KillResponse, error)     // callback for killing
	deliverFn func(agent *AgentHandle, text string) error // injectable for testing
	onEvent   func(StreamEvent)                           // called when agents change (for WS broadcast)
}

// DefaultSocketPath returns the Unix socket path for the daemon.
func DefaultSocketPath() string {
	return filepath.Join(os.TempDir(), "ws-relay.sock")
}

// NewDaemon creates a new relay daemon.
func NewDaemon() *Daemon {
	d := &Daemon{
		agents: make(map[string]*AgentHandle),
	}
	d.deliverFn = d.tmuxDeliver
	return d
}

// SetSpawnHandler sets the callback invoked when an agent requests a spawn.
func (d *Daemon) SetSpawnHandler(fn func(SpawnRequest) (SpawnResponse, error)) {
	d.onSpawn = fn
}

// SetKillHandler sets the callback invoked when an agent requests a kill.
func (d *Daemon) SetKillHandler(fn func(KillRequest) (KillResponse, error)) {
	d.onKill = fn
}

// SetEventHandler sets the callback invoked when agent state changes.
// Used to push events to WebSocket subscribers.
func (d *Daemon) SetEventHandler(fn func(StreamEvent)) {
	d.onEvent = fn
}

// emitEvent fires the event callback if set.
func (d *Daemon) emitEvent(event StreamEvent) {
	if d.onEvent != nil {
		d.onEvent(event)
	}
}

// enrichedStatus builds an AgentStatus with all fields from an AgentHandle.
// Caller must hold at least d.mu.RLock.
func (d *Daemon) enrichedStatus(handle *AgentHandle) AgentStatus {
	return AgentStatus{
		ID:       handle.ID,
		Status:   handle.Status,
		Agent:    handle.Agent,
		ParentID: handle.ParentID,
		Session:  handle.Session,
		PaneID:   handle.PaneID,
	}
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
		// Try exact match first
		if _, ok := d.agents[target]; ok {
			return d.deliver(target, msg)
		}
		// Resolve short name relative to sender: if "auth" sends to "Explore",
		// look up "auth/Explore"
		if sender, ok := d.agents[msg.From]; ok {
			qualified := sender.ID + "/" + target
			if _, ok := d.agents[qualified]; ok {
				log.Printf("[daemon] resolved short name %q -> %q (sender: %s)", target, qualified, msg.From)
				return d.deliver(qualified, msg)
			}
		}
		return d.deliver(target, msg) // will fail with "not found" error
	}
}

// deliver sends a message to an agent using the configured deliverFn.
func (d *Daemon) deliver(agentID string, msg Message) error {
	agent, ok := d.agents[agentID]
	if !ok {
		return fmt.Errorf("agent %q not found", agentID)
	}

	prefix := "[relay from " + msg.From + "] (" + string(msg.Type) + ") "
	text := prefix + msg.Content

	return d.deliverFn(agent, text)
}

// tmuxDeliver is the default deliverFn that sends text via tmux send-keys.
// Routes through SSH for remote agents.
func (d *Daemon) tmuxDeliver(agent *AgentHandle, text string) error {
	target := agent.Session
	if target == "" {
		target = "ws/" + agent.ID
	}

	// Verify the target session exists before sending
	check := runOnHost(agent.Host, "tmux", "has-session", "-t", target)
	if err := check.Run(); err != nil {
		return fmt.Errorf("pane %q not reachable for agent %q: %w", target, agent.ID, err)
	}

	// Retry up to 2 times on failure with 500ms backoff
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		cmd := runOnHost(agent.Host, "tmux", "send-keys", "-t", target, text, "Enter")
		if out, err := cmd.CombinedOutput(); err != nil {
			lastErr = fmt.Errorf("tmux send-keys to %q (agent %q): %s", target, agent.ID, string(out))
			log.Printf("[daemon] deliver attempt %d/%d failed: %v", attempt+1, 3, lastErr)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		log.Printf("[daemon] delivered message to agent %q via tmux (target: %s)", agent.ID, target)
		return nil
	}
	return lastErr
}

// runOnHost builds an exec.Cmd that runs locally (host="") or via SSH.
// This is a relay-local copy to avoid a dependency cycle with libs/core.
func runOnHost(host string, name string, args ...string) *exec.Cmd {
	if host == "" {
		return exec.Command(name, args...)
	}
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, name)
	for _, arg := range args {
		parts = append(parts, shellEscapeArg(arg))
	}
	remoteCmd := ""
	for i, p := range parts {
		if i > 0 {
			remoteCmd += " "
		}
		remoteCmd += p
	}
	return exec.Command("ssh", "-o", "BatchMode=yes", host, remoteCmd)
}

// shellEscapeArg wraps a string in single quotes if it contains shell-special chars.
func shellEscapeArg(s string) string {
	safe := true
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '/' || c == ':' || c == '@' || c == '%' || c == '+') {
			safe = false
			break
		}
	}
	if safe && s != "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// ListAgents returns all registered agents.
func (d *Daemon) ListAgents() []AgentStatus {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var result []AgentStatus
	for _, a := range d.agents {
		result = append(result, d.enrichedStatus(a))
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
	PaneID   string `json:"pane_id,omitempty"`
	WorkDir  string `json:"work_dir,omitempty"`
}

// HandleAction processes a single envelope and returns the response.
// Used by both Unix socket and WebSocket handlers.
func (d *Daemon) HandleAction(env Envelope) (any, error) {
	switch env.Action {
	case "send":
		var msg Message
		if err := json.Unmarshal(env.Payload, &msg); err != nil {
			return nil, err
		}
		if err := d.Route(msg); err != nil {
			return nil, err
		}
		return map[string]string{"status": "delivered"}, nil

	case "spawn":
		var req SpawnRequest
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			return nil, err
		}
		if d.onSpawn == nil {
			return nil, fmt.Errorf("no spawn handler configured")
		}
		return d.onSpawn(req)

	case "status":
		var req StatusRequest
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			return nil, err
		}
		if req.AgentID == "all" || req.AgentID == "" {
			return d.ListAgents(), nil
		}
		d.mu.RLock()
		defer d.mu.RUnlock()
		if a, ok := d.agents[req.AgentID]; ok {
			return AgentStatus{ID: a.ID, Status: a.Status, Agent: a.Agent}, nil
		}
		return nil, fmt.Errorf("agent %q not found", req.AgentID)

	case "kill":
		var req KillRequest
		if err := json.Unmarshal(env.Payload, &req); err != nil {
			return nil, err
		}
		if d.onKill == nil {
			return nil, fmt.Errorf("no kill handler configured")
		}
		kr, err := d.onKill(req)
		if err != nil {
			return nil, err
		}
		for _, killedID := range kr.Killed {
			d.Unregister(killedID)
			d.emitEvent(StreamEvent{
				Type: "agent_killed",
				Data: AgentStatus{ID: killedID, Status: "exited"},
			})
		}
		return kr, nil

	case "register":
		var reg RegisterPayload
		if err := json.Unmarshal(env.Payload, &reg); err != nil {
			return nil, err
		}
		d.mu.Lock()
		if existing, ok := d.agents[reg.AgentID]; ok {
			existing.Agent = reg.Agent
			existing.ParentID = reg.ParentID
			existing.Status = "alive"
			if reg.PaneID != "" {
				existing.PaneID = reg.PaneID
			}
			if reg.WorkDir != "" {
				existing.WorktreePath = reg.WorkDir
			}
		} else {
			d.agents[reg.AgentID] = &AgentHandle{
				ID:           reg.AgentID,
				ParentID:     reg.ParentID,
				Agent:        reg.Agent,
				WorktreePath: reg.WorkDir,
				Session:      "ws/" + reg.AgentID,
				PaneID:       reg.PaneID,
				Status:       "alive",
			}
		}
		status := d.enrichedStatus(d.agents[reg.AgentID])
		d.mu.Unlock()
		log.Printf("[daemon] registered agent %q (parent: %q, pane: %q)", reg.AgentID, reg.ParentID, reg.PaneID)
		d.emitEvent(StreamEvent{Type: "agent_spawned", Data: status})
		return map[string]string{"status": "registered"}, nil

	case "unregister":
		var reg RegisterPayload
		if err := json.Unmarshal(env.Payload, &reg); err != nil {
			return nil, err
		}
		d.Unregister(reg.AgentID)
		d.emitEvent(StreamEvent{
			Type: "agent_killed",
			Data: AgentStatus{ID: reg.AgentID, Status: "exited", Agent: reg.Agent},
		})
		return map[string]string{"status": "unregistered"}, nil

	default:
		return nil, fmt.Errorf("unknown action %q", env.Action)
	}
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

		resp, err := d.HandleAction(env)
		if err != nil {
			encoder.Encode(map[string]string{"error": err.Error()})
		} else {
			encoder.Encode(resp)
		}
	}
}
