package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/kencrim/grimoire/libs/core"
	"github.com/kencrim/grimoire/libs/relay"
	qrcode "github.com/skip2/go-qrcode"
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
		wsPort, _ := cmd.Flags().GetInt("ws-port")
		tsnetEnabled, _ := cmd.Flags().GetBool("tsnet")
		tsnetHostname, _ := cmd.Flags().GetString("tsnet-hostname")

		// Check if already running
		conn, err := net.Dial("unix", socketPath)
		if err == nil {
			conn.Close()
			return fmt.Errorf("daemon already running at %s", socketPath)
		}

		// If not foreground, re-exec ourselves in the background
		if !foreground {
			logFile, _ := os.Create(filepath.Join(os.TempDir(), "ws-daemon.log"))
			bgArgs := []string{"daemon", "start", "--foreground"}
			if wsPort > 0 {
				bgArgs = append(bgArgs, fmt.Sprintf("--ws-port=%d", wsPort))
			}
			if tsnetEnabled {
				bgArgs = append(bgArgs, "--tsnet", fmt.Sprintf("--tsnet-hostname=%s", tsnetHostname))
			}
			bgCmd := exec.Command(wsBin(), bgArgs...)
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
					if wsPort > 0 {
						fmt.Printf("WebSocket server on port %d\n", wsPort)
					}
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
		// Save real stderr before redirect so tsnet can print auth URLs to the terminal
		realStderr := os.Stderr
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
			var deadIDs []string
			for _, node := range tree.Nodes {
				// Check if the tmux session is still alive (SSH for remote)
				alive := false
				if node.Session != "" {
					check := core.RunOnHost(node.Host, "tmux", "has-session", "-t", node.Session)
					alive = check.Run() == nil
				}

				if alive {
					daemon.Register(&relay.AgentHandle{
						ID:           node.ID,
						ParentID:     node.ParentID,
						Agent:        node.Agent,
						WorktreePath: node.WorkDir,
						Session:      node.Session,
						Host:         node.Host,
						Status:       "alive",
					})
					rehydrated++
				} else {
					log.Printf("[daemon] skipped dead agent %q", node.ID)
					deadIDs = append(deadIDs, node.ID)
				}
			}

			// Prune dead entries from state.json so they don't persist across restarts
			if len(deadIDs) > 0 {
				for _, id := range deadIDs {
					delete(tree.Nodes, id)
				}
				if err := tree.Save(); err != nil {
					log.Printf("[daemon] warning: could not prune state.json: %v", err)
				} else {
					log.Printf("[daemon] pruned %d dead agent(s) from state.json", len(deadIDs))
				}
			}

			if rehydrated > 0 {
				log.Printf("[daemon] rehydrated %d agent(s) from state.json", rehydrated)
			}
		}

		// Wire up spawn handler — creates workstreams from agents or mobile app
		daemon.SetSpawnHandler(func(req relay.SpawnRequest) (relay.SpawnResponse, error) {
			var wsName string
			if req.ParentID != "" {
				wsName = req.ParentID + "/" + req.Name
			} else {
				wsName = req.Name
			}

			agent := req.Agent
			if agent == "" {
				agent = "claude"
			}

			log.Printf("[daemon] spawn requested: %s (parent: %q, repo: %q, agent: %s)", wsName, req.ParentID, req.Repo, agent)

			if err := createWorkstream(wsName, agent, req.Task, "", req.Repo, socketPath); err != nil {
				return relay.SpawnResponse{}, err
			}

			return relay.SpawnResponse{
				AgentID: wsName,
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
				// Kill this workstream's tmux session (routes through SSH for remote)
				if node.Session != "" {
					core.RunOnHost(node.Host, "tmux", "kill-session", "-t", node.Session).Run()
				}

				// Remove git worktree (local only)
				if err := removeWorktree(node); err != nil {
					log.Printf("[daemon] warning: %v", err)
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

		// Start WebSocket server if port specified
		var wsSrv *relay.WSServer
		var disco *relay.Discovery
		var tsNode *relay.TailscaleNode
		var monitor *relay.StatusMonitor
		if wsPort > 0 {
			wsSrv = relay.NewWSServer(daemon, core.DefaultStatePath())

			// Serve repo registry for mobile create form
			wsSrv.HandleFunc("/api/repos", func(w http.ResponseWriter, r *http.Request) {
				registry, err := core.LoadRepoRegistry(core.DefaultReposPath())
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				type repoEntry struct {
					Name string `json:"name"`
					Path string `json:"path"`
				}
				var repos []repoEntry
				for _, repo := range registry.Repos {
					repos = append(repos, repoEntry{Name: repo.Name, Path: repo.Path})
				}
				if repos == nil {
					repos = []repoEntry{}
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(repos)
			})

			// Accept image uploads from mobile — saves to /tmp, returns file path
			wsSrv.HandleFunc("/api/upload", func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					http.Error(w, "POST only", http.StatusMethodNotAllowed)
					return
				}

				// 10 MB max
				r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
				if err := r.ParseMultipartForm(10 << 20); err != nil {
					http.Error(w, "file too large or invalid form", http.StatusBadRequest)
					return
				}

				file, header, err := r.FormFile("image")
				if err != nil {
					http.Error(w, "missing 'image' field", http.StatusBadRequest)
					return
				}
				defer file.Close()

				// Determine extension from original filename
				ext := filepath.Ext(header.Filename)
				if ext == "" {
					ext = ".png"
				}

				tmp, err := os.CreateTemp("", "grimoire-img-*"+ext)
				if err != nil {
					http.Error(w, "create temp file: "+err.Error(), http.StatusInternalServerError)
					return
				}
				defer tmp.Close()

				if _, err := io.Copy(tmp, file); err != nil {
					http.Error(w, "write file: "+err.Error(), http.StatusInternalServerError)
					return
				}

				log.Printf("[daemon] image uploaded: %s (%s)", tmp.Name(), header.Filename)

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{"path": tmp.Name()})
			})

			daemon.SetEventHandler(func(event relay.StreamEvent) {
				wsSrv.NotifyStreams(event)
				// Send push notification for idle transitions
				if event.Type == "status_changed" {
					if status, ok := event.Data.(relay.AgentStatus); ok && status.Status == "idle" {
						wsSrv.Push.NotifyIdle(status)
					}
				}
			})

			// Start status monitor — polls agent panes for idle detection
			monitor = relay.NewStatusMonitor(daemon)
			monitor.Start()

			addr := fmt.Sprintf("0.0.0.0:%d", wsPort)
			go func() {
				if err := wsSrv.Listen(addr); err != nil {
					log.Printf("[daemon] WebSocket server error: %v", err)
				}
			}()

			// Persist WS port so `ws daemon connect` can read it
			os.WriteFile(relay.WSPortPath(), []byte(fmt.Sprintf("%d", wsPort)), 0o644)

			// Start embedded Tailscale node if requested
			if tsnetEnabled {
				tsNode = relay.NewTailscaleNode(tsnetHostname, wsPort, realStderr)

				// First-run auth requires foreground mode (or TS_AUTHKEY)
				if tsNode.NeedsAuth() && !foreground && os.Getenv("TS_AUTHKEY") == "" {
					log.Printf("[daemon] tsnet: first-time setup requires foreground mode")
					fmt.Fprintln(os.Stderr, "tsnet: first-time setup — run with --foreground to complete browser auth")
					fmt.Fprintln(os.Stderr, "  ws daemon start --foreground --tsnet")
					fmt.Fprintln(os.Stderr, "  (or set TS_AUTHKEY for headless auth)")
				} else {
					ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
					if _, err := tsNode.Up(ctx); err != nil {
						log.Printf("[daemon] tsnet failed (non-fatal): %v", err)
						tsNode = nil
					} else {
						// Serve WebSocket on the tsnet listener too
						ln, err := tsNode.Listen()
						if err != nil {
							log.Printf("[daemon] tsnet listen failed: %v", err)
						} else {
							go func() {
								if err := wsSrv.Serve(ln); err != nil {
									log.Printf("[daemon] tsnet serve error: %v", err)
								}
							}()
						}
					}
					cancel()
				}
			}

			// Start mDNS/Bonjour advertisement + Tailscale detection
			disco = relay.NewDiscovery(wsPort, wsSrv.Token())

			// If tsnet is active, override the Tailscale hostname with the tsnet FQDN
			if tsNode != nil && tsNode.FQDN() != "" {
				disco.SetTailscaleHost(tsNode.FQDN(), "")
			}

			if err := disco.StartMDNS(); err != nil {
				log.Printf("[daemon] mDNS failed (non-fatal): %v", err)
			}

			// Persist Tailscale hostname for `ws daemon connect`
			if tsHost := disco.TailscaleHost(); tsHost != "" {
				os.WriteFile(relay.TailscaleHostPath(), []byte(tsHost), 0o644)
			}

			log.Printf("[daemon] WebSocket server on port %d", wsPort)
			log.Printf("[daemon] Token: %s...", wsSrv.Token()[:8])
		}

		// Handle shutdown
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			log.Println("[daemon] shutting down...")
			if monitor != nil {
				monitor.Stop()
			}
			if disco != nil {
				disco.Close()
			}
			if tsNode != nil {
				tsNode.Close()
			}
			if wsSrv != nil {
				wsSrv.Close()
			}
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

var daemonConnectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Show QR code and connection details for Grimoire Mobile",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check daemon is running
		socketPath := relay.DefaultSocketPath()
		conn, err := net.Dial("unix", socketPath)
		if err != nil {
			return fmt.Errorf("daemon is not running — start it with: ws daemon start --ws-port 8077")
		}
		conn.Close()

		// Read saved token
		tokenData, err := os.ReadFile(relay.TokenPath())
		if err != nil {
			return fmt.Errorf("no auth token found — was the daemon started with --ws-port?")
		}
		token := strings.TrimSpace(string(tokenData))

		// Read saved WS port
		portData, err := os.ReadFile(relay.WSPortPath())
		if err != nil {
			return fmt.Errorf("no WebSocket port found — was the daemon started with --ws-port?")
		}
		var port int
		fmt.Sscanf(strings.TrimSpace(string(portData)), "%d", &port)
		if port == 0 {
			return fmt.Errorf("invalid WebSocket port")
		}

		// Detect IP addresses
		var tailscaleIP string
		var lanIPs []string

		// Only trust Tailscale IP if the tunnel is actually up
		if err := exec.Command("tailscale", "status", "--peers=false").Run(); err == nil {
			if out, err := exec.Command("tailscale", "ip", "-4").Output(); err == nil {
				tailscaleIP = strings.TrimSpace(string(out))
			}
		}

		ifaces, _ := net.Interfaces()
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, _ := iface.Addrs()
			for _, addr := range addrs {
				if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
					ip := ipnet.IP.String()
					// Skip Tailscale IP and link-local from LAN list
					if ip != tailscaleIP && !strings.HasPrefix(ip, "100.") {
						lanIPs = append(lanIPs, ip)
					}
				}
			}
		}

		// Pick the best address for the QR code (prefer LAN — phone is likely on same WiFi)
		primaryIP := "localhost"
		if len(lanIPs) > 0 {
			primaryIP = lanIPs[0]
		} else if tailscaleIP != "" {
			primaryIP = tailscaleIP
		}

		uri := fmt.Sprintf("grimoire://%s:%d?token=%s", primaryIP, port, token)

		// Generate QR code
		qr, err := qrcode.New(uri, qrcode.Medium)
		if err != nil {
			return fmt.Errorf("QR code generation failed: %w", err)
		}

		// Print everything to stdout
		fmt.Println()
		fmt.Println("  Scan with Grimoire Mobile:")
		fmt.Println()
		fmt.Print(qr.ToSmallString(false))
		fmt.Println()

		// Read saved Tailscale hostname
		var tailscaleHost string
		if tsData, err := os.ReadFile(relay.TailscaleHostPath()); err == nil {
			tailscaleHost = strings.TrimSpace(string(tsData))
		}

		if tailscaleHost != "" {
			fmt.Printf("  Tailscale:  %s:%d\n", tailscaleHost, port)
		} else if tailscaleIP != "" {
			fmt.Printf("  Tailscale:  %s:%d\n", tailscaleIP, port)
		}
		for _, ip := range lanIPs {
			fmt.Printf("  LAN:        %s:%d\n", ip, port)
		}
		fmt.Printf("  mDNS:       _grimoire._tcp (auto-discovered on LAN)\n")
		fmt.Println()
		fmt.Printf("  Token:      %s\n", token)
		fmt.Println()

		return nil
	},
}

func init() {
	daemonStartCmd.Flags().Bool("foreground", false, "Run in foreground (used internally)")
	daemonStartCmd.Flags().MarkHidden("foreground")
	daemonStartCmd.Flags().Int("ws-port", 8077, "Port for WebSocket server (0 = disabled)")
	daemonStartCmd.Flags().Bool("tsnet", false, "Enable embedded Tailscale node for remote access")
	daemonStartCmd.Flags().String("tsnet-hostname", "grimoire", "Hostname for the tsnet node on the tailnet")
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
	daemonCmd.AddCommand(daemonConnectCmd)
	rootCmd.AddCommand(daemonCmd)
}
