package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/kencrim/grimoire/libs/core"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show status of all workstreams",
	RunE: func(cmd *cobra.Command, args []string) error {
		tree, err := core.LoadTree(core.DefaultStatePath())
		if err != nil {
			return err
		}

		if len(tree.Nodes) == 0 {
			fmt.Println("No workstreams.")
			return nil
		}

		// Check which tmux sessions are actually alive
		tmuxList := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
		out, _ := tmuxList.Output()
		liveSessions := make(map[string]bool)
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			liveSessions[line] = true
		}

		fmt.Printf("%-20s %-10s %-10s %-30s %s\n", "WORKSTREAM", "AGENT", "STATUS", "BRANCH", "SESSION")
		fmt.Printf("%-20s %-10s %-10s %-30s %s\n", "----------", "-----", "------", "------", "-------")

		for _, node := range tree.Nodes {
			status := string(node.Status)
			if !liveSessions[node.Session] {
				status = "dead"
			}

			fmt.Printf("%-20s %-10s %-10s %-30s %s\n",
				node.ID,
				node.Agent,
				status,
				node.Branch,
				node.Session,
			)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
