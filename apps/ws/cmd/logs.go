package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/kencrim/grimoire/libs/core"
	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs <name>",
	Short: "Tail an agent's terminal output",
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

		// Capture last 50 lines of the pane
		capture := exec.Command("tmux", "capture-pane", "-p", "-t", node.Session, "-S", "-50")
		capture.Stdout = os.Stdout
		capture.Stderr = os.Stderr
		return capture.Run()
	},
}

func init() {
	rootCmd.AddCommand(logsCmd)
}
