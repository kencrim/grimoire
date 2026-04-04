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
	WorkDir   string     `json:"work_dir"`                  // Worktree or workspace path
	RepoDir   string     `json:"repo_dir,omitempty"`        // Git repo root that owns this worktree
	Session   string     `json:"session"`                   // tmux session name
	PaneID    string     `json:"pane_id,omitempty"` // tmux pane ID (e.g. %5)
	Color     string     `json:"color,omitempty"`   // Border color for tmux pane
	Shader    string     `json:"shader,omitempty"`  // Background shader name
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`

	// Remote-only fields
	Workspace string `json:"workspace,omitempty"` // Coder workspace name
	Host      string `json:"host,omitempty"`      // SSH host for remote
}

// WorkstreamTheme pairs a shader with its semantic border and background colors.
type WorkstreamTheme struct {
	Shader string // shader filename (e.g. "starfield.glsl")
	Border string // Catppuccin Mocha hex color for tmux border
	Tint   string // subtle background tint for tmux pane
	Label  string // human-readable name
}

// WorkstreamThemes maps shaders to colors from the Catppuccin Mocha palette.
var WorkstreamThemes = []WorkstreamTheme{
	{"starfield.glsl", "#b4befe", "#1e1e3a", "starfield"},              // lavender — night sky
	{"inside-the-matrix.glsl", "#a6e3a1", "#1e2e1e", "matrix"},        // green — matrix rain
	{"sparks-from-fire.glsl", "#fab387", "#2e1e1e", "embers"},          // peach — fire embers
	{"just-snow.glsl", "#89b4fa", "#1e2636", "snow"},                   // blue — winter cold
	{"gears-and-belts.glsl", "#f9e2af", "#2a2a1e", "gears"},            // yellow — mechanical brass
	{"cubes.glsl", "#cba6f7", "#261e30", "cubes"},                      // mauve — geometric purple
	{"animated-gradient-shader.glsl", "#94e2d5", "#1e2e2a", "gradient"}, // teal — flowing gradient
}

// AssignTheme picks a theme from the rotation based on node count.
func AssignTheme(existingCount int) WorkstreamTheme {
	return WorkstreamThemes[existingCount%len(WorkstreamThemes)]
}

// ThemeByShader looks up a theme by its shader filename.
// Falls back to the first theme if not found.
func ThemeByShader(shader string) WorkstreamTheme {
	for _, t := range WorkstreamThemes {
		if t.Shader == shader {
			return t
		}
	}
	return WorkstreamThemes[0]
}

// ThemeByBorder looks up a theme by its border color.
// Falls back to the first theme if not found.
func ThemeByBorder(border string) WorkstreamTheme {
	for _, t := range WorkstreamThemes {
		if t.Border == border {
			return t
		}
	}
	return WorkstreamThemes[0]
}
