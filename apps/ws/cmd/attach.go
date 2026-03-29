package cmd

import (
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/kencrim/grimoire/libs/core"
	"github.com/spf13/cobra"
)

var attachCmd = &cobra.Command{
	Use:   "attach <name>",
	Short: "Attach to a workstream's tmux session",
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

		// Swap the Ghostty background shader to match this workstream
		if node.Shader != "" {
			if err := applyShader(node.Shader); err != nil {
				log.Printf("[ws attach] warning: could not apply shader: %v", err)
			}
		}

		// Prefer PaneID for split panes, fall back to session
		target := node.PaneID
		if target == "" {
			target = node.Session
		}

		// If inside tmux, switch client (resolves pane IDs to their session automatically).
		// Otherwise attach to the session.
		if os.Getenv("TMUX") != "" {
			tmux := exec.Command("tmux", "switch-client", "-t", target)
			tmux.Stdin = os.Stdin
			tmux.Stdout = os.Stdout
			tmux.Stderr = os.Stderr
			return tmux.Run()
		}

		tmux := exec.Command("tmux", "attach-session", "-t", target)
		tmux.Stdin = os.Stdin
		tmux.Stdout = os.Stdout
		tmux.Stderr = os.Stderr
		return tmux.Run()
	},
}

func init() {
	rootCmd.AddCommand(attachCmd)
}
