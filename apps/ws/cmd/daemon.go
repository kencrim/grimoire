package cmd

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

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

		// Check if already running
		conn, err := net.Dial("unix", socketPath)
		if err == nil {
			conn.Close()
			return fmt.Errorf("daemon already running at %s", socketPath)
		}

		// Write PID file
		pidPath := filepath.Join(os.TempDir(), "ws-relay.pid")
		os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0o644)
		defer os.Remove(pidPath)

		daemon := relay.NewDaemon()

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
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
	rootCmd.AddCommand(daemonCmd)
}
