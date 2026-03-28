package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/kencrim/grimoire/libs/core"
	"github.com/spf13/cobra"
)

var addAgent string

var addCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Create a new workstream",
	Long: `Create a new workstream backed by a git worktree and tmux session.

Use slash-separated names to nest workstreams:
  ws add auth              # root workstream
  ws add auth/oauth        # child of auth

The agent flag determines which coding agent launches in the session.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		tree, err := core.LoadTree(core.DefaultStatePath())
		if err != nil {
			return err
		}

		// Parse parent from slash-separated name
		var parentID string
		parts := strings.Split(name, "/")
		if len(parts) > 1 {
			parentID = strings.Join(parts[:len(parts)-1], "/")
		}

		// Determine branch name and worktree path
		branch := "ws/" + name
		cwd, _ := os.Getwd()
		worktreePath := fmt.Sprintf("../%s", strings.ReplaceAll(name, "/", "-"))

		// Create git worktree
		gitAdd := exec.Command("git", "worktree", "add", worktreePath, "-b", branch)
		gitAdd.Dir = cwd
		if out, err := gitAdd.CombinedOutput(); err != nil {
			// Try without -b (branch may exist)
			gitAdd = exec.Command("git", "worktree", "add", worktreePath, branch)
			gitAdd.Dir = cwd
			if out2, err2 := gitAdd.CombinedOutput(); err2 != nil {
				return fmt.Errorf("git worktree add: %s\n%s", string(out), string(out2))
			}
			_ = out
		}

		// Resolve absolute path
		absPath := worktreePath
		if p, err := exec.Command("realpath", worktreePath).Output(); err == nil {
			absPath = strings.TrimSpace(string(p))
		}

		// tmux session name matches the workstream path
		sessionName := "ws/" + name

		// Create tmux session
		tmuxNew := exec.Command("tmux", "new-session", "-d", "-s", sessionName, "-c", absPath)
		if out, err := tmuxNew.CombinedOutput(); err != nil {
			return fmt.Errorf("tmux new-session: %s", string(out))
		}

		// Launch agent if specified
		if addAgent != "" {
			tmuxSend := exec.Command("tmux", "send-keys", "-t", sessionName, addAgent, "Enter")
			if out, err := tmuxSend.CombinedOutput(); err != nil {
				return fmt.Errorf("tmux send-keys: %s", string(out))
			}
		}

		// Add to tree
		node := &core.Node{
			ID:        name,
			Name:      parts[len(parts)-1],
			Branch:    branch,
			ParentID:  parentID,
			Type:      core.NodeTypeLocal,
			Status:    core.StatusRunning,
			Agent:     addAgent,
			WorkDir:   absPath,
			Session:   sessionName,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := tree.Add(node); err != nil {
			return err
		}
		if err := tree.Save(); err != nil {
			return err
		}

		fmt.Printf("Created workstream %q\n", name)
		fmt.Printf("  Branch:    %s\n", branch)
		fmt.Printf("  Worktree:  %s\n", absPath)
		fmt.Printf("  Session:   %s\n", sessionName)
		if addAgent != "" {
			fmt.Printf("  Agent:     %s\n", addAgent)
		}
		fmt.Printf("\nSwitch to it: ws switch %s\n", name)
		return nil
	},
}

func init() {
	addCmd.Flags().StringVarP(&addAgent, "agent", "a", "", "Agent to launch (claude, amp, codex)")
	rootCmd.AddCommand(addCmd)
}
