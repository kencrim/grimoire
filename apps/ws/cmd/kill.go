package cmd

import (
	"fmt"
	"os/exec"

	"github.com/kencrim/grimoire/libs/core"
	"github.com/spf13/cobra"
)

var killCmd = &cobra.Command{
	Use:   "kill <name>",
	Short: "Tear down a workstream and its children",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		tree, err := core.LoadTree(core.DefaultStatePath())
		if err != nil {
			return err
		}

		removed, err := tree.Remove(name)
		if err != nil {
			return err
		}

		for _, node := range removed {
			// Kill tmux session
			tmuxKill := exec.Command("tmux", "kill-session", "-t", node.Session)
			tmuxKill.Run() // ignore errors (session may not exist)

			// Remove git worktree
			if node.Type == core.NodeTypeLocal {
				gitRemove := exec.Command("git", "worktree", "remove", node.WorkDir, "--force")
				if out, err := gitRemove.CombinedOutput(); err != nil {
					fmt.Printf("  Warning: could not remove worktree %s: %s\n", node.WorkDir, string(out))
				}
			}

			fmt.Printf("  Removed %s\n", node.ID)
		}

		if err := tree.Save(); err != nil {
			return err
		}

		fmt.Printf("Killed %d workstream(s)\n", len(removed))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(killCmd)
}
