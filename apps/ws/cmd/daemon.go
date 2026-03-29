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
