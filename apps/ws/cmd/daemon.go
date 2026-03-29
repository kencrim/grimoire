package cmd

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kencrim/grimoire/libs/core"
	"github.com/kencrim/grimoire/libs/relay"
	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the relay daemon",
	Long:  "Start, stop, or check the status of the relay daemon.",
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the relay daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		socketPath := relay.DefaultSocketPath()
		foreground, _ := cmd.Flags().GetBool("foreground")

		// Check if already running
		conn, err := net.Dial("unix", socketPath)
		if err == nil {
			conn.Close()
			return fmt.Errorf("daemon already running at %s", socketPath)
		}

		// If not foreground, re-exec ourselves in the background
		if !foreground {
			logFile, _ := os.Create(filepath.Join(os.TempDir(), "ws-daemon.log"))
			bgCmd := exec.Command(wsBin(), "daemon", "start", "--foreground")
			bgCmd.Stdout = logFile
			bgCmd.Stderr = logFile
			bgCmd.Stdin = nil
			bgCmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
			if err := bgCmd.Start(); err != nil {
				return fmt.Errorf("start background daemon: %w", err)
			}
			// Wait for socket
			for i := 0; i < 20; i++ {
				time.Sleep(100 * time.Millisecond)
				c, err := net.Dial("unix", socketPath)
				if err == nil {
					c.Close()
					fmt.Printf("ws daemon started (pid %d)\n", bgCmd.Process.Pid)
					return nil
				}
			}
			return fmt.Errorf("daemon did not start within 2 seconds")
		}

		// Set up log file so daemon output is captured.
		// Redirect both log.* and fmt.Print* (stdout) to the log file,
		// since createWorkstream uses fmt.Printf for status output.
		logPath := filepath.Join(os.TempDir(), "ws-daemon.log")
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return fmt.Errorf("open log file: %w", err)
		}
		defer logFile.Close()
		log.SetOutput(logFile)
		os.Stdout = logFile
		os.Stderr = logFile

		// Write PID file
		pidPath := filepath.Join(os.TempDir(), "ws-relay.pid")
		os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0o644)
		defer os.Remove(pidPath)

		daemon := relay.NewDaemon()

		// Rehydrate agent registry from state.json
		if tree, err := core.LoadTree(core.DefaultStatePath()); err != nil {
			log.Printf("[daemon] warning: could not load state: %v", err)
		} else {
			rehydrated := 0
			for _, node := range tree.Nodes {
				// Check if the tmux session/pane is still alive
				alive := false
				if node.PaneID != "" {
					check := exec.Command("tmux", "display-message", "-t", node.PaneID, "-p", "")
					alive = check.Run() == nil
				} else if node.Session != "" {
					check := exec.Command("tmux", "has-session", "-t", node.Session)
					alive = check.Run() == nil
				}

				if alive {
					daemon.Register(&relay.AgentHandle{
						ID:           node.ID,
						ParentID:     node.ParentID,
						Agent:        node.Agent,
						WorktreePath: node.WorkDir,
						Session:      node.Session,
						PaneID:       node.PaneID,
						Status:       "alive",
					})
					rehydrated++
				} else {
					log.Printf("[daemon] skipped dead agent %q", node.ID)
				}
			}
			if rehydrated > 0 {
				log.Printf("[daemon] rehydrated %d agent(s) from state.json", rehydrated)
			}
		}

		// Wire up spawn handler — when an agent calls relay_spawn,
		// the daemon creates a child workstream
		daemon.SetSpawnHandler(func(req relay.SpawnRequest) (relay.SpawnResponse, error) {
			childName := req.ParentID + "/" + req.Name
			log.Printf("[daemon] spawn requested: %s (parent: %s)", childName, req.ParentID)

			if err := createWorkstream(childName, "amp", req.Task, socketPath); err != nil {
				return relay.SpawnResponse{}, err
			}

			return relay.SpawnResponse{
				AgentID: childName,
				Status:  "spawned",
			}, nil
		})

		// Wire up kill handler — when an agent calls relay_kill,
		// the daemon tears down the child workstream
		daemon.SetKillHandler(func(req relay.KillRequest) (relay.KillResponse, error) {
			log.Printf("[daemon] kill requested: %s", req.AgentID)

			tree, err := core.LoadTree(core.DefaultStatePath())
			if err != nil {
				return relay.KillResponse{}, fmt.Errorf("load state: %w", err)
			}

			removed, err := tree.Remove(req.AgentID)
			if err != nil {
				return relay.KillResponse{}, err
			}

			var killedIDs []string
			for _, node := range removed {
				// Kill tmux pane (prefer PaneID for split panes) or session
				if node.PaneID != "" {
					exec.Command("tmux", "kill-pane", "-t", node.PaneID).Run()
				} else {
					exec.Command("tmux", "kill-session", "-t", node.Session).Run()
				}

				// Remove git worktree
				if node.Type == core.NodeTypeLocal && node.WorkDir != "" {
					gitRemove := exec.Command("git", "worktree", "remove", node.WorkDir, "--force")
					if out, err := gitRemove.CombinedOutput(); err != nil {
						log.Printf("[daemon] warning: worktree remove %s: %s", node.WorkDir, string(out))
					}
				}

				killedIDs = append(killedIDs, node.ID)
				log.Printf("[daemon] killed agent %s", node.ID)
			}

			if err := tree.Save(); err != nil {
				log.Printf("[daemon] warning: could not save state after kill: %v", err)
			}

			return relay.KillResponse{
				Killed: killedIDs,
				Status: "killed",
			}, nil
		})

		// Handle shutdown
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			log.Println("[daemon] shutting down...")
			daemon.Close()
			os.Remove(socketPath)
			os.Exit(0)
		}()

		fmt.Printf("ws daemon starting on %s (pid %d)\n", socketPath, os.Getpid())
		return daemon.Listen(socketPath)
	},
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the relay daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		pidPath := filepath.Join(os.TempDir(), "ws-relay.pid")
		data, err := os.ReadFile(pidPath)
		if err != nil {
			return fmt.Errorf("daemon not running (no pid file)")
		}

		var pid int
		fmt.Sscanf(string(data), "%d", &pid)

		proc, err := os.FindProcess(pid)
		if err != nil {
			return fmt.Errorf("process %d not found", pid)
		}

		if err := proc.Signal(syscall.SIGTERM); err != nil {
			return fmt.Errorf("could not stop daemon (pid %d): %w", pid, err)
		}

		os.Remove(pidPath)
		os.Remove(relay.DefaultSocketPath())
		fmt.Printf("Stopped daemon (pid %d)\n", pid)
		return nil
	},
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check if the daemon is running",
	RunE: func(cmd *cobra.Command, args []string) error {
		socketPath := relay.DefaultSocketPath()
		conn, err := net.Dial("unix", socketPath)
		if err != nil {
			fmt.Println("Daemon is not running")
			return nil
		}
		conn.Close()

		pidPath := filepath.Join(os.TempDir(), "ws-relay.pid")
		data, _ := os.ReadFile(pidPath)
		fmt.Printf("Daemon is running (pid %s) at %s\n", string(data), socketPath)
		return nil
	},
}

func init() {
	daemonStartCmd.Flags().Bool("foreground", false, "Run in foreground (used internally)")
	daemonStartCmd.Flags().MarkHidden("foreground")
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
	rootCmd.AddCommand(daemonCmd)
}
