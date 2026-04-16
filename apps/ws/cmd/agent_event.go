package cmd

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/kencrim/grimoire/libs/relay"
	"github.com/spf13/cobra"
)

var agentEventCmd = &cobra.Command{
	Use:    "agent-event",
	Short:  "Report agent status change (called by Claude Code hooks)",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		agentID, _ := cmd.Flags().GetString("id")
		socketPath, _ := cmd.Flags().GetString("socket")
		status, _ := cmd.Flags().GetString("status")

		if agentID == "" || socketPath == "" || status == "" {
			return fmt.Errorf("--id, --socket, and --status are required")
		}

		conn, err := net.Dial("unix", socketPath)
		if err != nil {
			return fmt.Errorf("connect to daemon: %w", err)
		}
		defer conn.Close()

		payload, _ := json.Marshal(relay.TransitionPayload{
			AgentID: agentID,
			Status:  status,
		})
		json.NewEncoder(conn).Encode(relay.Envelope{Action: "transition", Payload: payload})

		var resp map[string]string
		json.NewDecoder(conn).Decode(&resp)

		if errMsg, ok := resp["error"]; ok {
			return fmt.Errorf("daemon: %s", errMsg)
		}
		return nil
	},
}

func init() {
	agentEventCmd.Flags().String("id", "", "Agent ID")
	agentEventCmd.Flags().String("socket", "", "Daemon socket path")
	agentEventCmd.Flags().String("status", "", "New status (idle, alive)")
	rootCmd.AddCommand(agentEventCmd)
}
