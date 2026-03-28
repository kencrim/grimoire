package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/kencrim/grimoire/libs/core"
	"github.com/spf13/cobra"
)

var switchCmd = &cobra.Command{
	Use:   "switch <name>",
	Short: "Switch to a workstream's tmux session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		tree, err := core.LoadTree(core.DefaultStatePath())
		if err != nil {
			return err
		}

		node, exists := tree.Nodes[name]
		if !exists {
			return fmt.Errorf("workstream %q not found", name)
		}

		// Check if we're inside tmux
		if os.Getenv("TMUX") != "" {
			tmux := exec.Command("tmux", "switch-client", "-t", node.Session)
			tmux.Stdin = os.Stdin
			tmux.Stdout = os.Stdout
			tmux.Stderr = os.Stderr
			return tmux.Run()
		}

		tmux := exec.Command("tmux", "attach-session", "-t", node.Session)
		tmux.Stdin = os.Stdin
		tmux.Stdout = os.Stdout
		tmux.Stderr = os.Stderr
		return tmux.Run()
	},
}

func init() {
	rootCmd.AddCommand(switchCmd)
}
