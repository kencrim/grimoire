// Package core provides shared types and utilities for grimoire CLI tools.
package core

import "time"

// NodeType represents where a workstream runs.
type NodeType string

const (
	NodeTypeLocal  NodeType = "local"  // Local git worktree
	NodeTypeRemote NodeType = "remote" // Remote devpod (Coder workspace)
)

// NodeStatus represents the current state of a workstream.
type NodeStatus string

const (
	StatusRunning NodeStatus = "running"
	StatusIdle    NodeStatus = "idle"
	StatusBlocked NodeStatus = "blocked"
	StatusDone    NodeStatus = "done"
)

// Node represents a single workstream in the DAG.
type Node struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Branch    string     `json:"branch"`
	ParentID  string     `json:"parent_id,omitempty"`
	Type      NodeType   `json:"type"`
	Status    NodeStatus `json:"status"`
	Agent     string     `json:"agent,omitempty"`  // claude, amp, codex
	WorkDir   string     `json:"work_dir"`         // Worktree or workspace path
	Session   string     `json:"session"`          // tmux session name
	PaneID    string     `json:"pane_id,omitempty"` // tmux pane ID (e.g. %5)
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`

	// Remote-only fields
	Workspace string `json:"workspace,omitempty"` // Coder workspace name
	Host      string `json:"host,omitempty"`      // SSH host for remote
}
