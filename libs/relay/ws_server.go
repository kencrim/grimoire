package relay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"nhooyr.io/websocket"
)

// WSServer handles WebSocket connections for the mobile companion app.
type WSServer struct {
	daemon  *Daemon
	token   string
	mux     *http.ServeMux
	treePath string // path to state.json for DAG snapshots
	Push    *PushService

	// Multiple HTTP servers (LAN + tsnet) sharing the same mux
	serversMu sync.Mutex
	servers   []*http.Server

	// Event subscribers for DAG state changes
	streamsMu   sync.Mutex
	streamsSubs map[chan StreamEvent]struct{}
}

// StreamEvent is pushed to /ws/streams subscribers when the DAG changes.
type StreamEvent struct {
	Type string      `json:"type"` // snapshot, agent_spawned, agent_killed, status_changed
	Data interface{} `json:"data"`
}

// PaneInputMsg is sent from the phone to write to a tmux pane.
type PaneInputMsg struct {
	Type string `json:"type"` // input, resize, special
	Data string `json:"data"` // text content or key name
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
}

// NewWSServer creates a WebSocket server attached to a daemon.
func NewWSServer(daemon *Daemon, treePath string) *WSServer {
	ws := &WSServer{
		daemon:      daemon,
		treePath:    treePath,
		mux:         http.NewServeMux(),
		streamsSubs: make(map[chan StreamEvent]struct{}),
		Push:        NewPushService(),
	}
	ws.token = ws.loadOrCreateToken()
	ws.mux.HandleFunc("/ws/streams", ws.requireAuth(ws.handleStreams))
	ws.mux.HandleFunc("/ws/panes/", ws.requireAuth(ws.handlePanes))
	ws.mux.HandleFunc("/ws/relay", ws.requireAuth(ws.handleRelay))
	ws.mux.HandleFunc("/api/health", ws.handleHealth)
	ws.mux.HandleFunc("/api/push-token", ws.Push.HandlePushToken(ws.requireAuth))
	return ws
}

// Token returns the auth token for QR code generation.
func (ws *WSServer) Token() string {
	return ws.token
}

// Serve starts serving on an existing net.Listener. The listener can come from
// net.Listen, tsnet.Server.Listen, or any other source. This allows the same
// HTTP mux to serve on multiple interfaces (e.g., LAN + Tailscale).
func (ws *WSServer) Serve(ln net.Listener) error {
	srv := &http.Server{Handler: ws.mux}
	ws.serversMu.Lock()
	ws.servers = append(ws.servers, srv)
	ws.serversMu.Unlock()
	log.Printf("[ws-server] serving on %s", ln.Addr())
	return srv.Serve(ln)
}

// Listen starts the HTTP/WebSocket server on the given address.
func (ws *WSServer) Listen(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("ws listen %s: %w", addr, err)
	}
	log.Printf("[ws-server] listening on %s", addr)
	return ws.Serve(ln)
}

// Close shuts down all HTTP servers.
func (ws *WSServer) Close() error {
	ws.serversMu.Lock()
	servers := ws.servers
	ws.servers = nil
	ws.serversMu.Unlock()
	var firstErr error
	for _, srv := range servers {
		if err := srv.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// NotifyStreams pushes an event to all /ws/streams subscribers.
func (ws *WSServer) NotifyStreams(event StreamEvent) {
	ws.streamsMu.Lock()
	defer ws.streamsMu.Unlock()
	for ch := range ws.streamsSubs {
		select {
		case ch <- event:
		default:
			// Drop if subscriber is slow
		}
	}
}

// TokenPath returns ~/.config/ws/daemon-token
func TokenPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ws", "daemon-token")
}

// WSPortPath returns ~/.config/ws/daemon-ws-port
func WSPortPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ws", "daemon-ws-port")
}

// TailscaleHostPath returns ~/.config/ws/daemon-ts-host
func TailscaleHostPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ws", "daemon-ts-host")
}

func (ws *WSServer) loadOrCreateToken() string {
	path := TokenPath()
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		return strings.TrimSpace(string(data))
	}
	// Generate new token
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte(token), 0o600)
	return token
}

// requireAuth wraps an HTTP handler with Bearer token authentication.
func (ws *WSServer) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check Authorization header
		auth := r.Header.Get("Authorization")
		if auth == "" {
			// Also check query param for WebSocket connections (some clients can't set headers)
			auth = "Bearer " + r.URL.Query().Get("token")
		}
		if auth != "Bearer "+ws.token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (ws *WSServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleStreams serves the /ws/streams endpoint — DAG state + live events.
func (ws *WSServer) handleStreams(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // allow any origin (phone app)
	})
	if err != nil {
		log.Printf("[ws-server] streams accept: %v", err)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx := r.Context()

	// Send initial DAG snapshot
	snapshot, err := ws.dagSnapshot()
	if err != nil {
		log.Printf("[ws-server] snapshot error: %v", err)
		return
	}
	event := StreamEvent{Type: "snapshot", Data: snapshot}
	data, _ := json.Marshal(event)
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		return
	}

	// Subscribe to events
	ch := make(chan StreamEvent, 32)
	ws.streamsMu.Lock()
	ws.streamsSubs[ch] = struct{}{}
	ws.streamsMu.Unlock()
	defer func() {
		ws.streamsMu.Lock()
		delete(ws.streamsSubs, ch)
		ws.streamsMu.Unlock()
	}()

	// Forward events to client
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(event)
			if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
				return
			}
		}
	}
}

// handlePanes serves /ws/panes/:id — bidirectional terminal I/O.
// Query param ?pane=agent|terminal selects which pane to stream.
// The phone acts as a viewport into the desktop terminal — it receives
// capture-pane snapshots at the desktop's native dimensions and renders
// them in xterm.js. No shadow sessions or resizing involved.
func (ws *WSServer) handlePanes(w http.ResponseWriter, r *http.Request) {
	// Extract pane ID from URL: /ws/panes/%5 or /ws/panes/auth
	paneRef := strings.TrimPrefix(r.URL.Path, "/ws/panes/")
	if paneRef == "" {
		http.Error(w, "pane ID required", http.StatusBadRequest)
		return
	}

	// Resolve pane — could be a pane ID (%5) or an agent ID (auth)
	agent := ws.resolvePane(paneRef)
	if agent == nil {
		http.Error(w, "pane not found", http.StatusNotFound)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.Printf("[ws-server] pane accept: %v", err)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Resolve the agent's pane ID — if not stored, get the first pane in the session
	agentPaneID := agent.PaneID
	if agentPaneID == "" && agent.Session != "" {
		agentPaneID = firstPaneInSession(agent.Session)
	}

	// Default to agent pane
	paneTarget := agentPaneID
	if paneTarget == "" {
		paneTarget = agent.Session
	}

	// If terminal tab requested, find the sibling pane
	paneSelector := r.URL.Query().Get("pane")
	if paneSelector == "terminal" && agent.Session != "" && agentPaneID != "" {
		if sibling := findSiblingPane(agent.Session, agentPaneID); sibling != "" {
			paneTarget = sibling
		}
	}

	// Stream pane output directly from the original session
	streamer := NewPaneStreamer(paneTarget)
	go streamer.StreamTo(ctx, conn)

	// Read input from phone
	for {
		_, msgData, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var input PaneInputMsg
		if err := json.Unmarshal(msgData, &input); err != nil {
			log.Printf("[ws-server] pane input decode: %v", err)
			continue
		}
		if err := streamer.HandleInput(input); err != nil {
			log.Printf("[ws-server] pane input error: %v", err)
		}
	}
}

// handleRelay bridges the existing envelope protocol over WebSocket.
func (ws *WSServer) handleRelay(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.Printf("[ws-server] relay accept: %v", err)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx := r.Context()

	for {
		_, msgData, err := conn.Read(ctx)
		if err != nil {
			return
		}

		var env Envelope
		if err := json.Unmarshal(msgData, &env); err != nil {
			writeJSON(ctx, conn, map[string]string{"error": "invalid envelope: " + err.Error()})
			continue
		}

		resp, err := ws.daemon.HandleAction(env)
		if err != nil {
			writeJSON(ctx, conn, map[string]string{"error": err.Error()})
			continue
		}
		writeJSON(ctx, conn, resp)
	}
}

func (ws *WSServer) resolvePane(ref string) *AgentHandle {
	ws.daemon.mu.RLock()
	defer ws.daemon.mu.RUnlock()

	// Try as agent ID first
	if agent, ok := ws.daemon.agents[ref]; ok {
		return agent
	}
	// Try as pane ID (e.g. %5)
	for _, agent := range ws.daemon.agents {
		if agent.PaneID == ref {
			return agent
		}
	}
	return nil
}

func (ws *WSServer) dagSnapshot() (interface{}, error) {
	agents := ws.daemon.ListAgents()

	type enrichedAgent struct {
		AgentStatus
		Color  string `json:"color,omitempty"`
		Shader string `json:"shader,omitempty"`
	}

	// Load state tree for color/shader metadata
	nodeColors := make(map[string]string)
	nodeShaders := make(map[string]string)
	if ws.treePath != "" {
		if data, err := os.ReadFile(ws.treePath); err == nil {
			var tree struct {
				Nodes map[string]struct {
					Color  string `json:"color"`
					Shader string `json:"shader"`
				} `json:"nodes"`
			}
			if json.Unmarshal(data, &tree) == nil {
				for id, n := range tree.Nodes {
					nodeColors[id] = n.Color
					nodeShaders[id] = n.Shader
				}
			}
		}
	}

	var result []enrichedAgent
	for _, a := range agents {
		ea := enrichedAgent{AgentStatus: a}
		ea.Color = nodeColors[a.ID]
		ea.Shader = nodeShaders[a.ID]
		result = append(result, ea)
	}
	return result, nil
}

// firstPaneInSession returns the first pane ID in a tmux session.
// This is the original pane (the agent pane) before any splits.
func firstPaneInSession(session string) string {
	cmd := exec.Command("tmux", "list-panes", "-t", session, "-F", "#{pane_id}")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}
	return ""
}

// findSiblingPane returns the other pane in a tmux session.
// Given the agent's pane ID, it finds the sibling (terminal) pane.
func findSiblingPane(session string, agentPaneID string) string {
	// List all panes in the session: output format "#{pane_id}" per line
	cmd := exec.Command("tmux", "list-panes", "-t", session, "-F", "#{pane_id}")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		paneID := strings.TrimSpace(line)
		if paneID != "" && paneID != agentPaneID {
			return paneID
		}
	}
	return ""
}

func writeJSON(ctx context.Context, conn *websocket.Conn, v interface{}) {
	data, _ := json.Marshal(v)
	conn.Write(ctx, websocket.MessageText, data)
}
