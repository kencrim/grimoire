package relay

import (
	"log"
	"regexp"
	"strings"
	"sync"
	"time"
)

// StatusMonitor polls agent tmux panes to detect idle/active transitions.
// When an agent's pane content stabilizes and the bottom of the pane matches
// a known idle prompt pattern, the agent transitions to "idle". When the
// content starts changing again, it transitions back to "alive".
type StatusMonitor struct {
	daemon   *Daemon
	interval time.Duration

	// How many consecutive unchanged polls before we check for idle patterns.
	// With a 2s interval and stableThreshold=2, that's ~4 seconds of stability.
	stableThreshold int

	mu     sync.Mutex
	states map[string]*agentPollState

	stopCh chan struct{}
}

type agentPollState struct {
	lastContent  string
	stableCount  int
	reportedIdle bool
}

// Per-agent-type idle prompt patterns. Checked against the last few non-empty
// lines of the stripped pane content. A match + content stability = idle.
var idlePatterns = map[string][]*regexp.Regexp{
	"claude": {
		regexp.MustCompile(`(?m)^\s*[❯>]\s*$`),    // Claude Code input prompt
		regexp.MustCompile(`(?m)^\s*\?\s+.+`),      // AskUserQuestion prompt
		regexp.MustCompile(`waiting for your`),      // "waiting for your response"
		regexp.MustCompile(`\(yes/no\)`),            // confirmation prompt
		regexp.MustCompile(`Has this been resolved`), // resolution check
	},
	"amp": {
		regexp.MustCompile(`(?m)^\s*[❯>$]\s*$`),
	},
	"codex": {
		regexp.MustCompile(`(?m)^\s*[❯>$]\s*$`),
	},
}

// NewStatusMonitor creates a monitor that polls agent panes for idle detection.
func NewStatusMonitor(daemon *Daemon) *StatusMonitor {
	return &StatusMonitor{
		daemon:          daemon,
		interval:        2 * time.Second,
		stableThreshold: 2,
		states:          make(map[string]*agentPollState),
		stopCh:          make(chan struct{}),
	}
}

// Start begins the polling loop in a background goroutine.
func (sm *StatusMonitor) Start() {
	go sm.run()
	log.Println("[status-monitor] started")
}

// Stop terminates the polling loop.
func (sm *StatusMonitor) Stop() {
	close(sm.stopCh)
}

func (sm *StatusMonitor) run() {
	ticker := time.NewTicker(sm.interval)
	defer ticker.Stop()

	for {
		select {
		case <-sm.stopCh:
			return
		case <-ticker.C:
			sm.pollAll()
		}
	}
}

func (sm *StatusMonitor) pollAll() {
	// Snapshot the current agents under daemon lock
	sm.daemon.mu.RLock()
	type agentSnapshot struct {
		id      string
		agent   string
		session string
		paneID  string
		host    string
		status  string
	}
	agents := make([]agentSnapshot, 0, len(sm.daemon.agents))
	for _, a := range sm.daemon.agents {
		agents = append(agents, agentSnapshot{
			id:      a.ID,
			agent:   a.Agent,
			session: a.Session,
			paneID:  a.PaneID,
			host:    a.Host,
			status:  a.Status,
		})
	}
	sm.daemon.mu.RUnlock()

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Clean up tracking state for agents that no longer exist
	activeIDs := make(map[string]bool, len(agents))
	for _, a := range agents {
		activeIDs[a.id] = true
	}
	for id := range sm.states {
		if !activeIDs[id] {
			delete(sm.states, id)
		}
	}

	for _, agent := range agents {
		if agent.status == "exited" {
			continue
		}

		state, ok := sm.states[agent.id]
		if !ok {
			state = &agentPollState{}
			sm.states[agent.id] = state
		}

		content := captureAgentPane(agent.host, agent.session, agent.paneID)
		if content == "" {
			continue
		}

		stripped := stripAnsi(content)

		if stripped == state.lastContent {
			state.stableCount++
		} else {
			state.stableCount = 0
			state.lastContent = stripped

			// Content is changing — if we previously reported idle, go back to alive
			if state.reportedIdle {
				state.reportedIdle = false
				sm.transitionAgent(agent.id, "alive")
			}
		}

		// Check for idle after sustained stability
		if state.stableCount >= sm.stableThreshold && !state.reportedIdle {
			if looksIdle(agent.agent, stripped) {
				state.reportedIdle = true
				sm.transitionAgent(agent.id, "idle")
			}
		}
	}
}

// looksIdle checks the last few non-empty lines of the pane content against
// known idle prompt patterns for the given agent type.
func looksIdle(agentType string, content string) bool {
	lines := strings.Split(strings.TrimRight(content, "\n "), "\n")

	// Gather the last 5 non-empty lines
	var tail []string
	for i := len(lines) - 1; i >= 0 && len(tail) < 5; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed != "" {
			tail = append(tail, trimmed)
		}
	}

	if len(tail) == 0 {
		return false
	}

	patterns, ok := idlePatterns[agentType]
	if !ok {
		patterns = idlePatterns["claude"] // fallback
	}

	for _, line := range tail {
		for _, pat := range patterns {
			if pat.MatchString(line) {
				return true
			}
		}
	}

	return false
}

func (sm *StatusMonitor) transitionAgent(agentID string, newStatus string) {
	sm.daemon.mu.Lock()
	agent, ok := sm.daemon.agents[agentID]
	if !ok {
		sm.daemon.mu.Unlock()
		return
	}
	oldStatus := agent.Status
	if oldStatus == newStatus {
		sm.daemon.mu.Unlock()
		return
	}
	agent.Status = newStatus
	status := sm.daemon.enrichedStatus(agent)
	sm.daemon.mu.Unlock()

	log.Printf("[status-monitor] agent %q: %s -> %s", agentID, oldStatus, newStatus)

	sm.daemon.emitEvent(StreamEvent{
		Type: "status_changed",
		Data: status,
	})
}

// captureAgentPane captures a tmux pane's content as plain text (no ANSI).
// Routes through SSH for remote agents.
func captureAgentPane(host, session, paneID string) string {
	target := paneID
	if target == "" {
		target = session
	}
	if target == "" {
		return ""
	}

	cmd := runOnHost(host, "tmux", "capture-pane", "-t", target, "-p")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}
