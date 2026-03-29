package relay

import (
	"sort"
	"strings"
	"sync"
	"testing"
)

// delivery records a single call to deliverFn.
type delivery struct {
	agentID string
	text    string
}

// testDaemon returns a Daemon with a mock deliverFn and a slice that records
// every delivery as "agentID:text".
func testDaemon() (*Daemon, *[]delivery) {
	var mu sync.Mutex
	delivered := &[]delivery{}
	d := NewDaemon()
	d.deliverFn = func(agent *AgentHandle, text string) error {
		mu.Lock()
		defer mu.Unlock()
		*delivered = append(*delivered, delivery{agentID: agent.ID, text: text})
		return nil
	}
	return d, delivered
}

func TestRegister(t *testing.T) {
	d, _ := testDaemon()
	d.Register(&AgentHandle{
		ID:     "auth",
		Agent:  "claude",
		Status: "alive",
	})

	agents := d.ListAgents()
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].ID != "auth" {
		t.Errorf("expected agent ID %q, got %q", "auth", agents[0].ID)
	}
	if agents[0].Agent != "claude" {
		t.Errorf("expected agent type %q, got %q", "claude", agents[0].Agent)
	}
	if agents[0].Status != "alive" {
		t.Errorf("expected status %q, got %q", "alive", agents[0].Status)
	}
}

func TestUnregister(t *testing.T) {
	d, _ := testDaemon()
	d.Register(&AgentHandle{ID: "auth", Agent: "claude", Status: "alive"})
	d.Unregister("auth")

	agents := d.ListAgents()
	if len(agents) != 0 {
		t.Fatalf("expected 0 agents after unregister, got %d", len(agents))
	}
}

func TestRoute_ExactMatch(t *testing.T) {
	d, delivered := testDaemon()
	d.Register(&AgentHandle{ID: "auth", Agent: "claude", Status: "alive"})

	err := d.Route(Message{
		From:    "user",
		To:      "auth",
		Type:    MsgTask,
		Content: "do stuff",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*delivered) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(*delivered))
	}
	dl := (*delivered)[0]
	if dl.agentID != "auth" {
		t.Errorf("expected delivery to %q, got %q", "auth", dl.agentID)
	}
	if !strings.Contains(dl.text, "do stuff") {
		t.Errorf("expected delivery text to contain %q, got %q", "do stuff", dl.text)
	}
	if !strings.Contains(dl.text, "[relay from user]") {
		t.Errorf("expected delivery text to contain relay prefix, got %q", dl.text)
	}
	if !strings.Contains(dl.text, "(task)") {
		t.Errorf("expected delivery text to contain message type, got %q", dl.text)
	}
}

func TestRoute_Parent(t *testing.T) {
	d, delivered := testDaemon()
	d.Register(&AgentHandle{ID: "root", Agent: "claude", Status: "alive"})
	d.Register(&AgentHandle{ID: "root/child", ParentID: "root", Agent: "claude", Status: "alive"})

	err := d.Route(Message{
		From:    "root/child",
		To:      "parent",
		Type:    MsgResult,
		Content: "done",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*delivered) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(*delivered))
	}
	if (*delivered)[0].agentID != "root" {
		t.Errorf("expected delivery to %q, got %q", "root", (*delivered)[0].agentID)
	}
}

func TestRoute_Siblings(t *testing.T) {
	d, delivered := testDaemon()
	d.Register(&AgentHandle{ID: "root/a", ParentID: "root", Agent: "claude", Status: "alive"})
	d.Register(&AgentHandle{ID: "root/b", ParentID: "root", Agent: "claude", Status: "alive"})
	d.Register(&AgentHandle{ID: "root/c", ParentID: "root", Agent: "claude", Status: "alive"})

	err := d.Route(Message{
		From:    "root/a",
		To:      "siblings",
		Type:    MsgStatus,
		Content: "hello siblings",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should deliver to root/b and root/c but NOT root/a
	ids := make([]string, len(*delivered))
	for i, dl := range *delivered {
		ids[i] = dl.agentID
	}
	sort.Strings(ids)

	if len(ids) != 2 {
		t.Fatalf("expected 2 deliveries, got %d: %v", len(ids), ids)
	}
	if ids[0] != "root/b" || ids[1] != "root/c" {
		t.Errorf("expected deliveries to [root/b, root/c], got %v", ids)
	}
}

func TestRoute_ShortName(t *testing.T) {
	d, delivered := testDaemon()
	d.Register(&AgentHandle{ID: "auth", Agent: "claude", Status: "alive"})
	d.Register(&AgentHandle{ID: "auth/explore", Agent: "claude", Status: "alive"})

	err := d.Route(Message{
		From:    "auth",
		To:      "explore",
		Type:    MsgTask,
		Content: "search for files",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(*delivered) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(*delivered))
	}
	if (*delivered)[0].agentID != "auth/explore" {
		t.Errorf("expected delivery to %q, got %q", "auth/explore", (*delivered)[0].agentID)
	}
}

func TestRoute_NotFound(t *testing.T) {
	d, _ := testDaemon()

	err := d.Route(Message{
		From:    "user",
		To:      "nope",
		Type:    MsgTask,
		Content: "hello",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent agent, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got %q", err.Error())
	}
}

func TestRoute_ParentNotFound(t *testing.T) {
	d, _ := testDaemon()
	d.Register(&AgentHandle{ID: "orphan", ParentID: "", Agent: "claude", Status: "alive"})

	err := d.Route(Message{
		From:    "orphan",
		To:      "parent",
		Type:    MsgTask,
		Content: "help",
	})
	if err == nil {
		t.Fatal("expected error for orphan sending to parent, got nil")
	}
	if !strings.Contains(err.Error(), "no parent") {
		t.Errorf("expected 'no parent' in error, got %q", err.Error())
	}
}

func TestKill_UnregistersAgents(t *testing.T) {
	d, _ := testDaemon()

	// Register parent and child
	d.Register(&AgentHandle{ID: "root", Agent: "amp", Status: "alive"})
	d.Register(&AgentHandle{ID: "root/child", ParentID: "root", Agent: "amp", Status: "alive"})

	// Set kill handler that returns both as killed
	d.SetKillHandler(func(req KillRequest) (KillResponse, error) {
		return KillResponse{
			Killed: []string{"root", "root/child"},
			Status: "killed",
		}, nil
	})

	// Simulate kill via handleConn would call onKill and unregister
	// Test the handler directly + manual unregister (matching handleConn logic)
	resp, err := d.onKill(KillRequest{AgentID: "root"})
	if err != nil {
		t.Fatalf("kill returned error: %v", err)
	}
	if len(resp.Killed) != 2 {
		t.Fatalf("expected 2 killed, got %d", len(resp.Killed))
	}

	// Simulate what handleConn does after onKill succeeds
	for _, id := range resp.Killed {
		d.Unregister(id)
	}

	agents := d.ListAgents()
	if len(agents) != 0 {
		t.Errorf("expected 0 agents after kill, got %d", len(agents))
	}
}

func TestHandleConn_Unregister(t *testing.T) {
	d, _ := testDaemon()
	d.Register(&AgentHandle{ID: "temp", Agent: "amp", Status: "alive"})

	// Verify registered
	if len(d.ListAgents()) != 1 {
		t.Fatal("expected 1 agent after register")
	}

	// Simulate unregister via handleConn
	d.Unregister("temp")

	if len(d.ListAgents()) != 0 {
		t.Error("expected 0 agents after unregister")
	}
}

func TestListAgents(t *testing.T) {
	d, _ := testDaemon()
	d.Register(&AgentHandle{ID: "a", Agent: "claude", Status: "alive"})
	d.Register(&AgentHandle{ID: "b", Agent: "amp", Status: "idle"})
	d.Register(&AgentHandle{ID: "c", Agent: "codex", Status: "exited"})

	agents := d.ListAgents()
	if len(agents) != 3 {
		t.Fatalf("expected 3 agents, got %d", len(agents))
	}

	// Build a map for order-independent assertion
	m := make(map[string]AgentStatus)
	for _, a := range agents {
		m[a.ID] = a
	}

	cases := []struct {
		id, agent, status string
	}{
		{"a", "claude", "alive"},
		{"b", "amp", "idle"},
		{"c", "codex", "exited"},
	}
	for _, tc := range cases {
		a, ok := m[tc.id]
		if !ok {
			t.Errorf("agent %q not found in ListAgents", tc.id)
			continue
		}
		if a.Agent != tc.agent {
			t.Errorf("agent %q: expected agent type %q, got %q", tc.id, tc.agent, a.Agent)
		}
		if a.Status != tc.status {
			t.Errorf("agent %q: expected status %q, got %q", tc.id, tc.status, a.Status)
		}
	}
}

func TestRegisterPayload_WorkDir(t *testing.T) {
	d, _ := testDaemon()
	d.Register(&AgentHandle{
		ID:           "worker",
		Agent:        "amp",
		WorktreePath: "/home/user/worktrees/worker",
		Status:       "alive",
	})

	agents := d.ListAgents()
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}

	// Verify WorktreePath is preserved (access via internal map since ListAgents doesn't return it)
	d.mu.RLock()
	defer d.mu.RUnlock()
	handle, ok := d.agents["worker"]
	if !ok {
		t.Fatal("agent 'worker' not found in registry")
	}
	if handle.WorktreePath != "/home/user/worktrees/worker" {
		t.Errorf("expected WorktreePath '/home/user/worktrees/worker', got %q", handle.WorktreePath)
	}
}
