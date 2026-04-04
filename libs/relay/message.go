// Package relay provides the message routing daemon and agent communication types.
package relay

import "time"

// MessageType categorizes relay messages.
type MessageType string

const (
	MsgTask     MessageType = "task"
	MsgResult   MessageType = "result"
	MsgStatus   MessageType = "status"
	MsgQuestion MessageType = "question"
	MsgError    MessageType = "error"
)

// Message is a relay message between agents or from a user.
type Message struct {
	From    string      `json:"from"`
	To      string      `json:"to"`
	Type    MessageType `json:"type"`
	Content string      `json:"content"`
	Time    time.Time   `json:"time"`
}

// SpawnRequest asks the daemon to create an agent.
// ParentID is optional — omit it for root workstreams.
type SpawnRequest struct {
	ParentID string `json:"parent_id,omitempty"`
	Name     string `json:"name"`
	Task     string `json:"task"`
	Context  string `json:"context,omitempty"`
	Repo     string `json:"repo,omitempty"`  // repo registry name (for root workstreams)
	Agent    string `json:"agent,omitempty"` // claude, amp, codex (default: claude)
}

// SpawnResponse is returned after spawning a child.
type SpawnResponse struct {
	AgentID string `json:"agent_id"`
	Status  string `json:"status"`
}

// StatusRequest asks for agent or DAG status.
type StatusRequest struct {
	AgentID string `json:"agent_id"` // "all" for full DAG
}

// AgentStatus is returned by status queries and streamed to mobile clients.
type AgentStatus struct {
	ID       string `json:"id"`
	Status   string `json:"status"` // alive, idle, exited
	Agent    string `json:"agent"`
	Task     string `json:"task,omitempty"`
	ParentID string `json:"parent_id,omitempty"`
	Session  string `json:"session,omitempty"`
	PaneID   string `json:"pane_id,omitempty"`
}

// SkillsRequest asks for available slash commands for an agent.
type SkillsRequest struct {
	AgentID string `json:"agent_id"`
}

// Skill represents a slash command available to an agent.
type Skill struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Source       string `json:"source"`                  // plugin, project, user
	ArgumentHint string `json:"argument_hint,omitempty"` // e.g. "<phase-number>", "[optional description]"
}

// KillRequest asks the daemon to terminate an agent and its descendants.
type KillRequest struct {
	AgentID string `json:"agent_id"`
}

// KillResponse is returned after killing an agent subtree.
type KillResponse struct {
	Killed []string `json:"killed"`
	Status string   `json:"status"`
}

// AmpJSONL is the JSONL format Amp expects on stdin.
type AmpJSONL struct {
	Type    string          `json:"type"`
	Message AmpJSONLMessage `json:"message"`
}

// AmpJSONLMessage is the message payload for Amp stdin.
type AmpJSONLMessage struct {
	Role    string           `json:"role"`
	Content []AmpJSONLContent `json:"content"`
}

// AmpJSONLContent is a content block in an Amp message.
type AmpJSONLContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// FormatForAmp converts a relay message into Amp's stdin JSONL format.
func FormatForAmp(msg Message) AmpJSONL {
	prefix := "[relay from " + msg.From + "] (" + string(msg.Type) + ") "
	return AmpJSONL{
		Type: "user",
		Message: AmpJSONLMessage{
			Role: "user",
			Content: []AmpJSONLContent{
				{Type: "text", Text: prefix + msg.Content},
			},
		},
	}
}
