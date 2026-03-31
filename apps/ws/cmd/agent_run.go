package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/kencrim/grimoire/libs/relay"
	"github.com/spf13/cobra"
)

var agentRunCmd = &cobra.Command{
	Use:    "agent-run",
	Short:  "Agent wrapper (launched by tmux, not directly)",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		agentID, _ := cmd.Flags().GetString("id")
		agentType, _ := cmd.Flags().GetString("agent")
		task, _ := cmd.Flags().GetString("task")
		socketPath, _ := cmd.Flags().GetString("socket")
		parentID, _ := cmd.Flags().GetString("parent")

		if agentID == "" || socketPath == "" {
			return fmt.Errorf("--id and --socket are required")
		}
		if agentType == "" {
			agentType = "claude"
		}

		// Discover our own tmux pane ID via TMUX_PANE env var
		// (tmux display-message -p returns the *active* pane, not ours)
		paneID := os.Getenv("TMUX_PANE")

		// Register with daemon
		workDir, _ := os.Getwd()
		conn, err := net.Dial("unix", socketPath)
		if err != nil {
			log.Printf("[agent-run] warning: could not connect to daemon: %v", err)
		} else {
			defer conn.Close()
			regPayload, _ := json.Marshal(relay.RegisterPayload{
				AgentID:  agentID,
				ParentID: parentID,
				Agent:    agentType,
				PaneID:   paneID,
				WorkDir:  workDir,
			})
			json.NewEncoder(conn).Encode(relay.Envelope{Action: "register", Payload: regPayload})
			var resp map[string]string
			json.NewDecoder(conn).Decode(&resp)
		}

		// Build agent command — interactive mode with real TTY
		var agentCmd *exec.Cmd
		switch agentType {
		case "amp":
			ampArgs := []string{"--dangerously-allow-all"}
			mcpConfig := fmt.Sprintf(`{"relay":{"command":"%s","args":["relay-server","--agent-id","%s","--socket","%s"]}}`,
				wsBin(), agentID, socketPath)
			ampArgs = append(ampArgs, "--mcp-config", mcpConfig)
			// Don't use -x — it exits after the task. Launch interactive and inject task via env.
			agentCmd = exec.Command("amp", ampArgs...)

		case "claude":
			claudeArgs := []string{"--dangerously-skip-permissions"}
			if task != "" {
				claudeArgs = append(claudeArgs, "-p", task)
			}
			agentCmd = exec.Command("claude", claudeArgs...)

		case "codex":
			codexArgs := []string{"--full-auto"}
			if task != "" {
				codexArgs = append(codexArgs, "-m", task)
			}
			agentCmd = exec.Command("codex", codexArgs...)

		default:
			return fmt.Errorf("unknown agent type: %s", agentType)
		}

		// Connect agent directly to the terminal — user sees the interactive TUI
		agentCmd.Stdin = os.Stdin
		agentCmd.Stdout = os.Stdout
		agentCmd.Stderr = os.Stderr

		if err := agentCmd.Start(); err != nil {
			return fmt.Errorf("start agent: %w", err)
		}

		// Forward signals to the agent
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			sig := <-sigCh
			agentCmd.Process.Signal(sig)
		}()

		return agentCmd.Wait()
	},
}

func init() {
	agentRunCmd.Flags().String("id", "", "Workstream ID")
	agentRunCmd.Flags().String("agent", "claude", "Agent type (claude, amp, codex)")
	agentRunCmd.Flags().String("task", "", "Task description")
	agentRunCmd.Flags().String("socket", "", "Daemon socket path")
	agentRunCmd.Flags().String("parent", "", "Parent workstream ID")
	rootCmd.AddCommand(agentRunCmd)
}
