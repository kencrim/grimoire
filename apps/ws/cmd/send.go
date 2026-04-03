package cmd

import (
	"fmt"
	"strings"

	"github.com/kencrim/grimoire/libs/core"
	"github.com/spf13/cobra"
)

var sendCmd = &cobra.Command{
	Use:   "send <name> <message>",
	Short: "Send a message to an agent",
	Long: `Inject a message into a running agent's terminal.

For Amp agents with --stream-json-input, writes JSONL to stdin.
For other agents, uses tmux send-keys as a fallback.`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		message := strings.Join(args[1:], " ")

		tree, err := core.LoadTree(core.DefaultStatePath())
		if err != nil {
			return err
		}

		node, exists := tree.Nodes[name]
		if !exists {
			return fmt.Errorf("workstream %q not found", name)
		}

		target := node.Session
		if target == "" {
			target = "ws/" + name
		}
		tmuxSend := core.RunOnHost(node.Host, "tmux", "send-keys", "-t", target, message, "Enter")
		if out, err := tmuxSend.CombinedOutput(); err != nil {
			return fmt.Errorf("send failed: %s", string(out))
		}

		fmt.Printf("Sent to %s: %s\n", name, message)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(sendCmd)
}
