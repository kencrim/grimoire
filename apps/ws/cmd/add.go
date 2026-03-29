package cmd

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kencrim/grimoire/libs/core"
	"github.com/kencrim/grimoire/libs/relay"
	"github.com/spf13/cobra"
)

// wsBin returns the absolute path to the current ws binary.
func wsBin() string {
	exe, err := os.Executable()
	if err != nil {
		return "ws"
	}
	return exe
}

var addAgent string
var addTask string

var addCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Create a new workstream",
	Long: `Create a new workstream backed by a git worktree and tmux session.

Use slash-separated names to nest workstreams:
  ws add auth --agent amp --task "Implement JWT auth"
  ws add auth/oauth --agent amp --task "Add OAuth2 support"

The parent is inferred from the slash-separated name.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		socketPath := relay.DefaultSocketPath()

		// Auto-start daemon if not running
		if err := ensureDaemon(socketPath); err != nil {
			return err
		}

		return createWorkstream(name, addAgent, addTask, socketPath)
	},
}

func createWorkstream(name, agent, task, socketPath string) error {
	tree, err := core.LoadTree(core.DefaultStatePath())
	if err != nil {
		return err
	}

	// Idempotency: if workstream already exists, just attach to it
	if existing, ok := tree.Nodes[name]; ok {
		fmt.Printf("Workstream %q already exists, attaching...\n", name)
		attachCmd := exec.Command("tmux", "switch-client", "-t", existing.Session)
		if err := attachCmd.Run(); err != nil {
			// switch-client fails outside tmux; try attach instead
			attachCmd = exec.Command("tmux", "attach-session", "-t", existing.Session)
			attachCmd.Stdin = os.Stdin
			attachCmd.Stdout = os.Stdout
			attachCmd.Stderr = os.Stderr
			return attachCmd.Run()
		}
		return nil
	}

	// Parse parent from slash-separated name
	var parentID string
	parts := strings.Split(name, "/")
	if len(parts) > 1 {
		parentID = strings.Join(parts[:len(parts)-1], "/")
	}

	// Determine branch and worktree path.
	// Use the parent's WorkDir as the git repo base when spawning children
	// (the daemon's cwd is not a git repo). For root workstreams, use the
	// caller's cwd since `ws add` is run from inside the repo.
	branch := "ws-" + strings.ReplaceAll(name, "/", "-")
	var repoDir string
	if parentID != "" {
		if parentNode, exists := tree.Nodes[parentID]; exists {
			repoDir = parentNode.WorkDir
		}
	}
	if repoDir == "" {
		repoDir, _ = os.Getwd()
	}

	// Worktree goes alongside the repo dir (sibling directory)
	worktreeName := strings.ReplaceAll(name, "/", "-")
	worktreePath := filepath.Join(filepath.Dir(repoDir), worktreeName)

	// Prune stale worktree registrations (e.g. directory was deleted but git still tracks it)
	gitPrune := exec.Command("git", "worktree", "prune")
	gitPrune.Dir = repoDir
	gitPrune.CombinedOutput()

	// If the worktree directory already exists, reuse it
	if _, statErr := os.Stat(worktreePath); statErr == nil {
		// Directory exists — just make sure it's a valid git worktree
		log.Printf("[ws add] reusing existing worktree at %s", worktreePath)
	} else {
		// Create git worktree — try new branch, then existing branch
		gitAdd := exec.Command("git", "worktree", "add", worktreePath, "-b", branch)
		gitAdd.Dir = repoDir
		if _, err := gitAdd.CombinedOutput(); err != nil {
			gitAdd = exec.Command("git", "worktree", "add", worktreePath, branch)
			gitAdd.Dir = repoDir
			if out2, err2 := gitAdd.CombinedOutput(); err2 != nil {
				return fmt.Errorf("git worktree add: %s", string(out2))
			}
		}
	}

	absPath := worktreePath

	// tmux session name
	sessionName := "ws/" + name

	// Determine if this is a child — if so, split a pane in the parent's session
	var paneID string
	if parentID != "" {
		parentNode, exists := tree.Nodes[parentID]
		if !exists {
			return fmt.Errorf("parent workstream %q not found", parentID)
		}
		// Split a pane in the parent's tmux session, capturing the new pane ID
		splitCmd := exec.Command("tmux", "split-window", "-d", "-P", "-F", "#{pane_id}",
			"-t", parentNode.Session,
			"-c", absPath,
			fmt.Sprintf("%s agent-run --id %s --agent %s --socket %s --parent %s",
				wsBin(), name, agent, socketPath, parentID))
		out, err := splitCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("tmux split-window: %s", string(out))
		}
		paneID = strings.TrimSpace(string(out))
		// Inject initial task after agent starts
		if task != "" {
			waitForPane(paneID)
			taskSend := exec.Command("tmux", "send-keys", "-t", paneID, task, "Enter")
			taskSend.CombinedOutput()
		}
	} else {
		// Root workstream — create or reuse tmux session
		checkSession := exec.Command("tmux", "has-session", "-t", sessionName)
		if checkSession.Run() == nil {
			// Session exists — get its pane ID
			getPaneCmd := exec.Command("tmux", "list-panes", "-t", sessionName, "-F", "#{pane_id}")
			if out, err := getPaneCmd.Output(); err == nil {
				paneID = strings.TrimSpace(strings.Split(string(out), "\n")[0])
			}
			log.Printf("[ws add] reusing existing tmux session %s (pane %s)", sessionName, paneID)
		} else {
			tmuxNew := exec.Command("tmux", "new-session", "-d", "-s", sessionName, "-c", absPath,
				"-P", "-F", "#{pane_id}")
			out, err := tmuxNew.CombinedOutput()
			if err != nil {
				return fmt.Errorf("tmux new-session: %s", string(out))
			}
			paneID = strings.TrimSpace(string(out))
		}

		// Split into agent (left, 65%) and terminal (right, 35%)
		// The initial pane becomes the agent pane; split-window creates the terminal pane
		splitCmd := exec.Command("tmux", "split-window", "-h", "-d",
			"-t", paneID,
			"-c", absPath,
			"-l", "35%")
		splitCmd.CombinedOutput()

		// Launch agent-run in the left pane (the original pane)
		if agent != "" {
			agentRunCmd := fmt.Sprintf("%s agent-run --id %s --agent %s --socket %s",
				wsBin(), name, agent, socketPath)
			tmuxSend := exec.Command("tmux", "send-keys", "-t", paneID, agentRunCmd, "Enter")
			if out, err := tmuxSend.CombinedOutput(); err != nil {
				return fmt.Errorf("tmux send-keys: %s", string(out))
			}

			// Inject initial task after agent starts
			if task != "" {
				waitForPane(paneID)
				taskSend := exec.Command("tmux", "send-keys", "-t", paneID, task, "Enter")
				taskSend.CombinedOutput()
			}
		}
	}

	// Assign a theme — children inherit from parent, roots get a new one
	var theme core.WorkstreamTheme
	var actualSession string // the tmux session this workstream lives in

	if parentID != "" {
		// Child: inherit parent's theme and session
		parentNode := tree.Nodes[parentID]
		if parentNode.Shader != "" {
			theme = core.ThemeByShader(parentNode.Shader)
		} else if parentNode.Color != "" {
			theme = core.ThemeByBorder(parentNode.Color)
		} else {
			theme = core.AssignTheme(len(tree.Nodes))
		}
		actualSession = parentNode.Session
	} else {
		// Root: assign a new theme
		theme = core.AssignTheme(len(tree.Nodes))
		actualSession = sessionName
	}

	// Apply tmux pane styling — border color + background tint
	if paneID != "" {
		// Enable border status line on the session
		exec.Command("tmux", "set", "-t", actualSession, "pane-border-status", "top").Run()
		exec.Command("tmux", "set", "-p", "-t", paneID, "pane-border-style", fmt.Sprintf("fg=%s", theme.Border)).Run()
		exec.Command("tmux", "set", "-p", "-t", paneID, "pane-active-border-style", fmt.Sprintf("fg=%s", theme.Border)).Run()
		// Background tint for visual identity
		exec.Command("tmux", "select-pane", "-t", paneID, "-P", fmt.Sprintf("bg=%s", theme.Tint)).Run()
	}

	// For root workstreams, also apply tint to the terminal pane (right side)
	if parentID == "" {
		listPanes := exec.Command("tmux", "list-panes", "-t", actualSession, "-F", "#{pane_id}")
		if out, err := listPanes.Output(); err == nil {
			panes := strings.Split(strings.TrimSpace(string(out)), "\n")
			for _, p := range panes {
				p = strings.TrimSpace(p)
				if p != "" && p != paneID {
					exec.Command("tmux", "select-pane", "-t", p, "-P", fmt.Sprintf("bg=%s", theme.Tint)).Run()
					exec.Command("tmux", "set", "-p", "-t", p, "pane-border-style", fmt.Sprintf("fg=%s", theme.Border)).Run()
					exec.Command("tmux", "set", "-p", "-t", p, "pane-active-border-style", fmt.Sprintf("fg=%s", theme.Border)).Run()
				}
			}
		}
	}

	// Add or update in tree
	node := &core.Node{
		ID:        name,
		Name:      parts[len(parts)-1],
		Branch:    branch,
		ParentID:  parentID,
		Type:      core.NodeTypeLocal,
		Status:    core.StatusRunning,
		Agent:     agent,
		WorkDir:   absPath,
		Session:   actualSession, // children live in parent's session
		PaneID:    paneID,
		Color:     theme.Border,
		Shader:    theme.Shader,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if existing, ok := tree.Nodes[name]; ok {
		node.CreatedAt = existing.CreatedAt
		tree.Nodes[name] = node
	} else if err := tree.Add(node); err != nil {
		return err
	}
	if err := tree.Save(); err != nil {
		return err
	}

	fmt.Printf("Created workstream %q\n", name)
	fmt.Printf("  Branch:    %s\n", branch)
	fmt.Printf("  Worktree:  %s\n", absPath)
	fmt.Printf("  Session:   %s\n", sessionName)
	if agent != "" {
		fmt.Printf("  Agent:     %s\n", agent)
	}
	if parentID != "" {
		fmt.Printf("  Parent:    %s (split pane)\n", parentID)
	}
	fmt.Printf("  Theme:     %s (border: %s)\n", theme.Label, theme.Border)

	// Apply shader and switch to the new workstream
	if node.Shader != "" {
		applyShader(node.Shader)
	}

	target := node.PaneID
	if target == "" {
		target = node.Session
	}

	if os.Getenv("TMUX") != "" {
		tmuxSwitch := exec.Command("tmux", "switch-client", "-t", target)
		tmuxSwitch.Stdin = os.Stdin
		tmuxSwitch.Stdout = os.Stdout
		tmuxSwitch.Stderr = os.Stderr
		tmuxSwitch.Run()
	} else {
		tmuxAttach := exec.Command("tmux", "attach-session", "-t", node.Session)
		tmuxAttach.Stdin = os.Stdin
		tmuxAttach.Stdout = os.Stdout
		tmuxAttach.Stderr = os.Stderr
		tmuxAttach.Run()
	}

	return nil
}

// waitForPane polls until a tmux pane is ready (has a running process).
// Times out after 15 seconds.
func waitForPane(paneID string) {
	for i := 0; i < 30; i++ {
		time.Sleep(500 * time.Millisecond)
		check := exec.Command("tmux", "display-message", "-t", paneID, "-p", "#{pane_pid}")
		if check.Run() == nil {
			return
		}
	}
	log.Printf("[ws add] warning: pane %s not ready after 15s, proceeding anyway", paneID)
}

// ensureDaemon starts the daemon in the background if it's not running.
func ensureDaemon(socketPath string) error {
	conn, err := net.Dial("unix", socketPath)
	if err == nil {
		conn.Close()
		return nil // already running
	}

	// Start daemon in background
	daemonCmd := exec.Command(wsBin(), "daemon", "start")
	daemonCmd.Stdout = nil
	daemonCmd.Stderr = nil
	if err := daemonCmd.Start(); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}

	// Wait for socket to appear
	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)
		conn, err := net.Dial("unix", socketPath)
		if err == nil {
			conn.Close()
			return nil
		}
	}
	return fmt.Errorf("daemon did not start within 2 seconds")
}

func init() {
	addCmd.Flags().StringVarP(&addAgent, "agent", "a", "amp", "Agent to launch (amp, claude, codex)")
	addCmd.Flags().StringVarP(&addTask, "task", "t", "", "Task description for the agent")
	rootCmd.AddCommand(addCmd)
}
