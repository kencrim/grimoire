package cmd

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"

	"github.com/kencrim/grimoire/libs/core"
	"github.com/kencrim/grimoire/libs/relay"
	"github.com/spf13/cobra"
)

var killCmd = &cobra.Command{
	Use:   "kill <name>",
	Short: "Tear down a workstream and its children",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		background, _ := cmd.Flags().GetBool("bg")

		tree, err := core.LoadTree(core.DefaultStatePath())
		if err != nil {
			return err
		}

		if _, exists := tree.Nodes[name]; !exists {
			return fmt.Errorf("workstream %q not found", name)
		}

		// If we're inside the session we're about to kill,
		// switch out and re-run the kill in the background from the new session.
		if !background && os.Getenv("TMUX") != "" {
			currentSession, _ := exec.Command("tmux", "display-message", "-p", "#{session_name}").Output()
			current := strings.TrimSpace(string(currentSession))

			node := tree.Nodes[name]
			if current == node.Session {
				// Detach from tmux first, then run kill in background
				exec.Command("tmux", "run-shell", "-b",
					fmt.Sprintf("sleep 0.3 && %s kill --bg %s", wsBin(), name)).Run()
				exec.Command("tmux", "detach-client").Run()
				return nil
			}
		}

		// Reset shader to gradient
		applyShader("animated-gradient-shader.glsl")

		removed, err := tree.Remove(name)
		if err != nil {
			return err
		}

		for _, node := range removed {
			// Kill tmux session
			tmuxKill := exec.Command("tmux", "kill-session", "-t", node.Session)
			tmuxKill.Run()

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

		// Notify daemon to unregister killed agents (best-effort)
		socketPath := relay.DefaultSocketPath()
		conn, dialErr := net.Dial("unix", socketPath)
		if dialErr == nil {
			enc := json.NewEncoder(conn)
			dec := json.NewDecoder(conn)
			for _, node := range removed {
				regPayload, _ := json.Marshal(relay.RegisterPayload{AgentID: node.ID})
				enc.Encode(relay.Envelope{Action: "unregister", Payload: regPayload})
				var resp map[string]string
				dec.Decode(&resp)
			}
			conn.Close()
		}

		if !background {
			fmt.Printf("Killed %d workstream(s)\n", len(removed))
		}
		return nil
	},
}

func init() {
	killCmd.Flags().Bool("bg", false, "Run in background (used internally)")
	killCmd.Flags().MarkHidden("bg")
	rootCmd.AddCommand(killCmd)
}
