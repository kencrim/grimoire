package cmd

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kencrim/grimoire/libs/core"
	"github.com/kencrim/grimoire/libs/relay"
	"github.com/spf13/cobra"
)

// wsBin returns the absolute path to the current ws binary.
func wsBin() string {
	exe, err := os.Executable()
	if err != nil {
		return "ws"
	}
	return exe
}

var addAgent string
var addTask string
var addBranch string
var addParent string
var addOn string
var addRepo string

var addCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Create a new workstream",
	Long: `Create a new workstream backed by a git worktree and tmux session.

Nest workstreams using --parent or slash-separated names:
  ws add auth --task "Implement JWT auth"
  ws add oauth --parent auth --task "Add OAuth2 support"
  ws add auth/oauth --task "Add OAuth2 support"

Children get their own tmux session, worktree, and branch (forked from
the parent's branch). They inherit the parent's visual theme.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		socketPath := relay.DefaultSocketPath()

		// Resolve parent: --parent flag vs slash-separated name
		if addParent != "" && strings.Contains(name, "/") {
			return fmt.Errorf("cannot use --parent with a slash-separated name; use one or the other")
		}
		if addParent != "" {
			// --parent flag: construct the full ID
			name = addParent + "/" + name
		}

		// Auto-start daemon if not running
		if err := ensureDaemon(socketPath); err != nil {
			return err
		}

		// Remote workstream creation
		if addOn != "" {
			return createRemoteWorkstream(name, addAgent, addTask, addOn, socketPath)
		}

		return createWorkstream(name, addAgent, addTask, addBranch, addRepo, socketPath)
	},
}

func createWorkstream(name, agent, task, branchOverride, repo, socketPath string) error {
	tree, err := core.LoadTree(core.DefaultStatePath())
	if err != nil {
		return err
	}

	// Idempotency: if workstream already exists, just attach to it
	if existing, ok := tree.Nodes[name]; ok {
		fmt.Printf("Workstream %q already exists, attaching...\n", name)
		attachCmd := exec.Command("tmux", "switch-client", "-t", existing.Session)
		if err := attachCmd.Run(); err != nil {
			// switch-client fails outside tmux; try attach instead
			attachCmd = exec.Command("tmux", "attach-session", "-t", existing.Session)
			attachCmd.Stdin = os.Stdin
			attachCmd.Stdout = os.Stdout
			attachCmd.Stderr = os.Stderr
			return attachCmd.Run()
		}
		return nil
	}

	// Parse parent from slash-separated name
	var parentID string
	parts := strings.Split(name, "/")
	if len(parts) > 1 {
		parentID = strings.Join(parts[:len(parts)-1], "/")
	}

	// Validate parent exists when specified
	var parentNode *core.Node
	if parentID != "" {
		var exists bool
		parentNode, exists = tree.Nodes[parentID]
		if !exists {
			return fmt.Errorf("parent workstream %q not found", parentID)
		}
	}

	// Determine branch name
	var branch string
	if branchOverride != "" {
		branch = branchOverride
	} else {
		branch = "ws-" + strings.ReplaceAll(name, "/", "-")
	}

	// Determine repo dir: smart resolution that works from anywhere.
	repoDir, err := resolveRepoDir(parentNode, repo)
	if err != nil {
		return err
	}

	// Worktree goes alongside the repo dir (sibling directory)
	worktreeName := strings.ReplaceAll(name, "/", "-")
	worktreePath := filepath.Join(filepath.Dir(repoDir), worktreeName)

	// Prune stale worktree registrations
	gitPrune := exec.Command("git", "worktree", "prune")
	gitPrune.Dir = repoDir
	gitPrune.CombinedOutput()

	// Create the worktree
	if _, statErr := os.Stat(worktreePath); statErr == nil {
		log.Printf("[ws add] reusing existing worktree at %s", worktreePath)
	} else if branchOverride != "" {
		// User specified an existing branch — validate before creating worktree
		verify := exec.Command("git", "rev-parse", "--verify", "refs/heads/"+branch)
		verify.Dir = repoDir
		if _, err := verify.CombinedOutput(); err != nil {
			verifyRemote := exec.Command("git", "rev-parse", "--verify", "refs/remotes/origin/"+branch)
			verifyRemote.Dir = repoDir
			if _, err2 := verifyRemote.CombinedOutput(); err2 != nil {
				return fmt.Errorf("branch %q does not exist locally or on origin\n"+
					"Create it first with: git branch %s\n"+
					"Or check remote branches with: git branch -r", branch, branch)
			}
		}
		gitAdd := exec.Command("git", "worktree", "add", worktreePath, branch)
		gitAdd.Dir = repoDir
		if out, err := gitAdd.CombinedOutput(); err != nil {
			errMsg := string(out)
			if strings.Contains(errMsg, "already checked out") {
				return fmt.Errorf("branch %q is already checked out in another worktree\n%s",
					branch, strings.TrimSpace(errMsg))
			}
			return fmt.Errorf("git worktree add: %s", errMsg)
		}
	} else if parentNode != nil {
		// Child: branch off the parent's branch
		gitAdd := exec.Command("git", "worktree", "add", worktreePath, "-b", branch, parentNode.Branch)
		gitAdd.Dir = repoDir
		if _, err := gitAdd.CombinedOutput(); err != nil {
			// Fall back to using existing branch
			gitAdd = exec.Command("git", "worktree", "add", worktreePath, branch)
			gitAdd.Dir = repoDir
			if out2, err2 := gitAdd.CombinedOutput(); err2 != nil {
				return fmt.Errorf("git worktree add: %s", string(out2))
			}
		}
	} else {
		// Root: branch off HEAD
		gitAdd := exec.Command("git", "worktree", "add", worktreePath, "-b", branch)
		gitAdd.Dir = repoDir
		if _, err := gitAdd.CombinedOutput(); err != nil {
			gitAdd = exec.Command("git", "worktree", "add", worktreePath, branch)
			gitAdd.Dir = repoDir
			if out2, err2 := gitAdd.CombinedOutput(); err2 != nil {
				return fmt.Errorf("git worktree add: %s", string(out2))
			}
		}
	}

	absPath := worktreePath

	// Every workstream gets its own tmux session
	sessionName := "ws/" + name

	// Create or reuse tmux session
	var agentPaneID string
	checkSession := exec.Command("tmux", "has-session", "-t", sessionName)
	if checkSession.Run() == nil {
		// Session exists — get its first pane ID
		getPaneCmd := exec.Command("tmux", "list-panes", "-t", sessionName, "-F", "#{pane_id}")
		if out, err := getPaneCmd.Output(); err == nil {
			agentPaneID = strings.TrimSpace(strings.Split(string(out), "\n")[0])
		}
		log.Printf("[ws add] reusing existing tmux session %s (pane %s)", sessionName, agentPaneID)
	} else {
		tmuxNew := exec.Command("tmux", "new-session", "-d", "-s", sessionName, "-c", absPath,
			"-P", "-F", "#{pane_id}")
		out, err := tmuxNew.CombinedOutput()
		if err != nil {
			return fmt.Errorf("tmux new-session: %s", string(out))
		}
		agentPaneID = strings.TrimSpace(string(out))
	}

	// Split into agent (left, 65%) and terminal (right, 35%)
	splitCmd := exec.Command("tmux", "split-window", "-h", "-d",
		"-t", agentPaneID,
		"-c", absPath,
		"-l", "35%")
	splitCmd.CombinedOutput()

	// Launch agent-run in the left pane
	if agent != "" {
		agentRunArgs := fmt.Sprintf("%s agent-run --id %s --agent %s --socket %s",
			wsBin(), name, agent, socketPath)
		if parentID != "" {
			agentRunArgs += fmt.Sprintf(" --parent %s", parentID)
		}
		tmuxSend := exec.Command("tmux", "send-keys", "-t", agentPaneID, agentRunArgs, "Enter")
		if out, err := tmuxSend.CombinedOutput(); err != nil {
			return fmt.Errorf("tmux send-keys: %s", string(out))
		}

		// Inject initial task after agent starts
		if task != "" {
			waitForPane(agentPaneID)
			taskSend := exec.Command("tmux", "send-keys", "-t", agentPaneID, task, "Enter")
			taskSend.CombinedOutput()
		}
	}

	// Assign theme — children inherit parent's, roots get a new one
	var theme core.WorkstreamTheme
	if parentNode != nil {
		if parentNode.Shader != "" {
			theme = core.ThemeByShader(parentNode.Shader)
		} else if parentNode.Color != "" {
			theme = core.ThemeByBorder(parentNode.Color)
		} else {
			theme = core.AssignTheme(len(tree.Nodes))
		}
	} else {
		theme = core.AssignTheme(len(tree.Nodes))
	}

	// Apply tmux pane styling to all panes in this session
	exec.Command("tmux", "set", "-t", sessionName, "pane-border-status", "top").Run()
	listPanes := exec.Command("tmux", "list-panes", "-t", sessionName, "-F", "#{pane_id}")
	if out, err := listPanes.Output(); err == nil {
		for _, p := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			exec.Command("tmux", "set", "-p", "-t", p, "pane-border-style", fmt.Sprintf("fg=%s", theme.Border)).Run()
			exec.Command("tmux", "set", "-p", "-t", p, "pane-active-border-style", fmt.Sprintf("fg=%s", theme.Border)).Run()
			exec.Command("tmux", "select-pane", "-t", p, "-P", fmt.Sprintf("bg=%s", theme.Tint)).Run()
		}
	}

	// Add or update in tree
	node := &core.Node{
		ID:        name,
		Name:      parts[len(parts)-1],
		Branch:    branch,
		ParentID:  parentID,
		Type:      core.NodeTypeLocal,
		Status:    core.StatusRunning,
		Agent:     agent,
		WorkDir:   absPath,
		RepoDir:   repoDir,
		Session:   sessionName,
		Color:     theme.Border,
		Shader:    theme.Shader,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if existing, ok := tree.Nodes[name]; ok {
		node.CreatedAt = existing.CreatedAt
		tree.Nodes[name] = node
	} else if err := tree.Add(node); err != nil {
		return err
	}
	if err := tree.Save(); err != nil {
		return err
	}

	fmt.Printf("Created workstream %q\n", name)
	fmt.Printf("  Branch:    %s\n", branch)
	fmt.Printf("  Worktree:  %s\n", absPath)
	fmt.Printf("  Session:   %s\n", sessionName)
	if agent != "" {
		fmt.Printf("  Agent:     %s\n", agent)
	}
	if parentID != "" {
		fmt.Printf("  Parent:    %s\n", parentID)
		if parentNode != nil {
			fmt.Printf("  Forked:    %s\n", parentNode.Branch)
		}
	}
	fmt.Printf("  Theme:     %s (border: %s)\n", theme.Label, theme.Border)

	// Apply shader and switch to the new workstream
	if node.Shader != "" {
		applyShader(node.Shader)
	}

	if os.Getenv("TMUX") != "" {
		tmuxSwitch := exec.Command("tmux", "switch-client", "-t", sessionName)
		tmuxSwitch.Stdin = os.Stdin
		tmuxSwitch.Stdout = os.Stdout
		tmuxSwitch.Stderr = os.Stderr
		tmuxSwitch.Run()
	} else {
		tmuxAttach := exec.Command("tmux", "attach-session", "-t", sessionName)
		tmuxAttach.Stdin = os.Stdin
		tmuxAttach.Stdout = os.Stdout
		tmuxAttach.Stderr = os.Stderr
		tmuxAttach.Run()
	}

	return nil
}

// waitForPane polls until a tmux pane is ready (has a running process).
// Times out after 15 seconds.
func waitForPane(paneID string) {
	for i := 0; i < 30; i++ {
		time.Sleep(500 * time.Millisecond)
		check := exec.Command("tmux", "display-message", "-t", paneID, "-p", "#{pane_pid}")
		if check.Run() == nil {
			return
		}
	}
	log.Printf("[ws add] warning: pane %s not ready after 15s, proceeding anyway", paneID)
}

// ensureDaemon starts the daemon in the background if it's not running.
func ensureDaemon(socketPath string) error {
	conn, err := net.Dial("unix", socketPath)
	if err == nil {
		conn.Close()
		return nil // already running
	}

	// Start daemon in background
	daemonCmd := exec.Command(wsBin(), "daemon", "start")
	daemonCmd.Stdout = nil
	daemonCmd.Stderr = nil
	if err := daemonCmd.Start(); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}

	// Wait for socket to appear
	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)
		conn, err := net.Dial("unix", socketPath)
		if err == nil {
			conn.Close()
			return nil
		}
	}
	return fmt.Errorf("daemon did not start within 2 seconds")
}

// createRemoteWorkstream creates a workstream on a registered remote host.
// The agent runs on the remote inside a tmux session accessed via SSH.
func createRemoteWorkstream(name, agent, task, remoteName, socketPath string) error {
	tree, err := core.LoadTree(core.DefaultStatePath())
	if err != nil {
		return err
	}

	// Idempotency: if workstream already exists, just attach
	if existing, ok := tree.Nodes[name]; ok {
		fmt.Printf("Workstream %q already exists, attaching...\n", name)
		if existing.Type == core.NodeTypeRemote && existing.Host != "" {
			sshAttach := exec.Command("ssh", "-t", existing.Host, "tmux", "attach-session", "-t", existing.Session)
			sshAttach.Stdin = os.Stdin
			sshAttach.Stdout = os.Stdout
			sshAttach.Stderr = os.Stderr
			return sshAttach.Run()
		}
		attachCmd := exec.Command("tmux", "switch-client", "-t", existing.Session)
		if err := attachCmd.Run(); err != nil {
			attachCmd = exec.Command("tmux", "attach-session", "-t", existing.Session)
			attachCmd.Stdin = os.Stdin
			attachCmd.Stdout = os.Stdout
			attachCmd.Stderr = os.Stderr
			return attachCmd.Run()
		}
		return nil
	}

	// Resolve the remote host
	registry, err := core.LoadRegistry(core.DefaultRemotesPath())
	if err != nil {
		return fmt.Errorf("load remotes: %w", err)
	}
	remote, ok := registry.Get(remoteName)
	if !ok {
		return fmt.Errorf("remote %q not found (see: ws remote list)", remoteName)
	}

	sshHost := remote.SSHHost
	workDir := remote.WorkDir
	sessionName := "ws/" + name

	// Create tmux session on remote
	tmuxNew := core.RunOnHost(sshHost, "tmux", "new-session", "-d", "-s", sessionName,
		"-c", workDir, "-P", "-F", "#{pane_id}")
	out, err := tmuxNew.CombinedOutput()
	if err != nil {
		// Check if session already exists on remote
		checkSession := core.RunOnHost(sshHost, "tmux", "has-session", "-t", sessionName)
		if checkSession.Run() == nil {
			log.Printf("[ws add] reusing existing remote tmux session %s", sessionName)
		} else {
			return fmt.Errorf("remote tmux new-session: %s", string(out))
		}
	}
	agentPaneID := strings.TrimSpace(string(out))

	// Split into agent (left, 65%) and terminal (right, 35%)
	splitCmd := core.RunOnHost(sshHost, "tmux", "split-window", "-h", "-d",
		"-t", agentPaneID, "-c", workDir, "-l", "35%")
	splitCmd.CombinedOutput()

	// Launch agent directly (no ws agent-run wrapper on remote)
	if agent != "" {
		var agentLaunchCmd string
		switch agent {
		case "claude":
			agentLaunchCmd = "claude --dangerously-skip-permissions"
		case "amp":
			agentLaunchCmd = "amp --dangerously-allow-all"
		case "codex":
			agentLaunchCmd = "codex --full-auto"
		default:
			return fmt.Errorf("unknown agent type: %s", agent)
		}

		tmuxSend := core.RunOnHost(sshHost, "tmux", "send-keys", "-t", agentPaneID, agentLaunchCmd, "Enter")
		if out, err := tmuxSend.CombinedOutput(); err != nil {
			return fmt.Errorf("remote tmux send-keys: %s", string(out))
		}

		// Inject initial task after agent starts
		if task != "" {
			waitForRemotePane(sshHost, agentPaneID)
			taskSend := core.RunOnHost(sshHost, "tmux", "send-keys", "-t", agentPaneID, task, "Enter")
			taskSend.CombinedOutput()
		}
	}

	// Assign theme (for mobile app + list display, even though Ghostty shaders are local-only)
	theme := core.AssignTheme(len(tree.Nodes))

	// Build and save the node
	parts := strings.Split(name, "/")
	node := &core.Node{
		ID:        name,
		Name:      parts[len(parts)-1],
		Type:      core.NodeTypeRemote,
		Status:    core.StatusRunning,
		Agent:     agent,
		WorkDir:   workDir,
		Session:   sessionName,
		PaneID:    agentPaneID,
		Color:     theme.Border,
		Shader:    theme.Shader,
		Host:      sshHost,
		Workspace: remoteName,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := tree.Add(node); err != nil {
		return err
	}
	if err := tree.Save(); err != nil {
		return err
	}

	fmt.Printf("Created remote workstream %q\n", name)
	fmt.Printf("  Remote:    %s (%s)\n", remoteName, sshHost)
	fmt.Printf("  WorkDir:   %s\n", workDir)
	fmt.Printf("  Session:   %s\n", sessionName)
	if agent != "" {
		fmt.Printf("  Agent:     %s\n", agent)
	}
	fmt.Printf("  Theme:     %s (border: %s)\n", theme.Label, theme.Border)

	// Attach interactively via SSH
	sshAttach := exec.Command("ssh", "-t", sshHost, "tmux", "attach-session", "-t", sessionName)
	sshAttach.Stdin = os.Stdin
	sshAttach.Stdout = os.Stdout
	sshAttach.Stderr = os.Stderr
	sshAttach.Run()

	return nil
}

// waitForRemotePane polls until a remote tmux pane is ready.
func waitForRemotePane(host, paneID string) {
	for i := 0; i < 30; i++ {
		time.Sleep(500 * time.Millisecond)
		check := core.RunOnHost(host, "tmux", "display-message", "-t", paneID, "-p", "#{pane_pid}")
		if check.Run() == nil {
			return
		}
	}
	log.Printf("[ws add] warning: remote pane %s not ready after 15s, proceeding anyway", paneID)
}

// resolveRepoDir determines the git repository root for a new workstream.
// Resolution order:
//  1. --repo flag → look up in repo registry
//  2. Parent node → inherit RepoDir (fall back to WorkDir for old nodes)
//  3. cwd → auto-detect via git rev-parse --show-toplevel
//  4. Single repo in registry → use it
//  5. Multiple repos → error listing choices
//  6. No repos → error with guidance
func resolveRepoDir(parentNode *core.Node, repoFlag string) (string, error) {
	// 1. Explicit --repo flag
	if repoFlag != "" {
		registry, err := core.LoadRepoRegistry(core.DefaultReposPath())
		if err != nil {
			return "", fmt.Errorf("load repo registry: %w", err)
		}
		repo, ok := registry.Get(repoFlag)
		if !ok {
			return "", fmt.Errorf("repo %q not found (see: ws repo list)", repoFlag)
		}
		return repo.Path, nil
	}

	// 2. Inherit from parent
	if parentNode != nil {
		if parentNode.RepoDir != "" {
			return parentNode.RepoDir, nil
		}
		// Backward compat: old nodes don't have RepoDir
		return parentNode.WorkDir, nil
	}

	// 3. Auto-detect from cwd
	cwd, err := os.Getwd()
	if err == nil {
		gitTopLevel := exec.Command("git", "rev-parse", "--show-toplevel")
		gitTopLevel.Dir = cwd
		if out, err := gitTopLevel.Output(); err == nil {
			return strings.TrimSpace(string(out)), nil
		}
	}

	// 4-5. Fall back to repo registry
	registry, err := core.LoadRepoRegistry(core.DefaultReposPath())
	if err == nil {
		if len(registry.Repos) == 1 {
			for _, repo := range registry.Repos {
				return repo.Path, nil
			}
		}
		if len(registry.Repos) > 1 {
			var names []string
			for name := range registry.Repos {
				names = append(names, name)
			}
			return "", fmt.Errorf("multiple repos registered; specify one with --repo:\n  %s",
				strings.Join(names, "\n  "))
		}
	}

	// 6. Nothing available
	return "", fmt.Errorf("not inside a git repo and no repos registered\n" +
		"Register one with: ws repo add <name> /path/to/repo")
}

func init() {
	addCmd.Flags().StringVarP(&addAgent, "agent", "a", "claude", "Agent to launch (claude, amp, codex)")
	addCmd.Flags().StringVarP(&addTask, "task", "t", "", "Task description for the agent")
	addCmd.Flags().StringVarP(&addBranch, "branch", "b", "", "Use an existing git branch (instead of creating ws-<name>)")
	addCmd.Flags().StringVarP(&addParent, "parent", "p", "", "Parent workstream (child branches off parent's branch)")
	addCmd.Flags().StringVar(&addOn, "on", "", "Remote host to create workstream on (from ws remote list)")
	addCmd.Flags().StringVar(&addRepo, "repo", "", "Repository to create workstream in (from ws repo list)")
	rootCmd.AddCommand(addCmd)
}
