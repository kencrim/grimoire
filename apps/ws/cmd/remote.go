package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/kencrim/grimoire/libs/core"
	"github.com/spf13/cobra"
)

var remoteCmd = &cobra.Command{
	Use:   "remote",
	Short: "Manage remote workspace hosts",
	Long: `Register, list, and remove remote Coder workspaces.

  ws remote add my-dev-pod     Create and register a Coder workspace
  ws remote list               Show registered remotes
  ws remote remove my-dev-pod  Unregister (and optionally delete) a remote`,
}

var remoteAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Create a Coder workspace and register it as a remote host",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		registry, err := core.LoadRegistry(core.DefaultRemotesPath())
		if err != nil {
			return err
		}

		if _, exists := registry.Hosts[name]; exists {
			return fmt.Errorf("remote %q already registered", name)
		}

		// Step 1: List available templates
		fmt.Println("Fetching Coder templates...")
		templates, err := listCoderTemplates()
		if err != nil {
			return fmt.Errorf("list templates: %w", err)
		}
		if len(templates) == 0 {
			return fmt.Errorf("no Coder templates available")
		}

		// Step 2: Pick template via fzf
		templateName, err := pickTemplate(templates)
		if err != nil {
			return fmt.Errorf("template selection: %w", err)
		}
		if templateName == "" {
			return fmt.Errorf("no template selected")
		}
		fmt.Printf("Using template: %s\n", templateName)

		// Step 3: Create workspace (interactive — lets Coder prompt for parameters)
		fmt.Printf("Creating workspace %q...\n", name)
		createCmd := exec.Command("coder", "create", name, "--template", templateName)
		createCmd.Stdin = os.Stdin
		createCmd.Stdout = os.Stdout
		createCmd.Stderr = os.Stderr
		if err := createCmd.Run(); err != nil {
			return fmt.Errorf("coder create: %w", err)
		}

		// Step 4: Configure SSH access
		fmt.Println("Configuring SSH...")
		configSSH := exec.Command("coder", "config-ssh", "-y")
		configSSH.Stdout = os.Stdout
		configSSH.Stderr = os.Stderr
		if err := configSSH.Run(); err != nil {
			return fmt.Errorf("coder config-ssh: %w", err)
		}

		// Derive SSH host: coder config-ssh creates entries as "<workspace>.coder"
		sshHost := name + ".coder"

		// Step 5: Detect working directory
		fmt.Println("Detecting workspace directory...")
		pwdCmd := exec.Command("coder", "ssh", name, "pwd")
		pwdOut, err := pwdCmd.Output()
		if err != nil {
			return fmt.Errorf("detect work dir: %w (is the workspace running?)", err)
		}
		workDir := strings.TrimSpace(string(pwdOut))
		if workDir == "" {
			workDir = "/home/coder"
		}

		// Step 6: Save to registry
		host := &core.RemoteHost{
			Name:     name,
			SSHHost:  sshHost,
			WorkDir:  workDir,
			Template: templateName,
			Created:  time.Now(),
		}
		if err := registry.Add(host); err != nil {
			return err
		}
		if err := registry.Save(); err != nil {
			return err
		}

		fmt.Printf("\nRegistered remote %q\n", name)
		fmt.Printf("  SSH Host:  %s\n", sshHost)
		fmt.Printf("  Work Dir:  %s\n", workDir)
		fmt.Printf("  Template:  %s\n", templateName)
		fmt.Println("\nCreate a workstream with: ws add <name> --on", name)
		return nil
	},
}

var remoteListCmd = &cobra.Command{
	Use:     "list",
	Short:   "Show registered remote hosts",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		registry, err := core.LoadRegistry(core.DefaultRemotesPath())
		if err != nil {
			return err
		}

		if len(registry.Hosts) == 0 {
			fmt.Println("No remotes registered. Add one with: ws remote add <name>")
			return nil
		}

		// Optionally fetch live workspace status from Coder
		statusMap := fetchCoderStatuses()

		fmt.Printf("%-20s %-25s %-30s %-15s %s\n", "NAME", "SSH HOST", "WORK DIR", "TEMPLATE", "STATUS")
		fmt.Printf("%-20s %-25s %-30s %-15s %s\n", "----", "--------", "--------", "--------", "------")
		for _, host := range registry.Hosts {
			status := statusMap[host.Name]
			if status == "" {
				status = "unknown"
			}
			fmt.Printf("%-20s %-25s %-30s %-15s %s\n",
				host.Name,
				host.SSHHost,
				host.WorkDir,
				host.Template,
				status,
			)
		}
		return nil
	},
}

var remoteRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Unregister a remote host",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		deleteWorkspace, _ := cmd.Flags().GetBool("delete")

		registry, err := core.LoadRegistry(core.DefaultRemotesPath())
		if err != nil {
			return err
		}

		if _, exists := registry.Hosts[name]; !exists {
			return fmt.Errorf("remote %q not found", name)
		}

		if deleteWorkspace {
			fmt.Printf("Deleting Coder workspace %q...\n", name)
			delCmd := exec.Command("coder", "delete", name, "-y")
			delCmd.Stdout = os.Stdout
			delCmd.Stderr = os.Stderr
			if err := delCmd.Run(); err != nil {
				fmt.Printf("Warning: could not delete workspace: %v\n", err)
			}
		}

		if err := registry.Remove(name); err != nil {
			return err
		}
		if err := registry.Save(); err != nil {
			return err
		}

		fmt.Printf("Removed remote %q\n", name)
		return nil
	},
}

// coderTemplate represents a template from `coder templates list --output json`.
type coderTemplate struct {
	Template struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"Template"`
}

func listCoderTemplates() ([]coderTemplate, error) {
	cmd := exec.Command("coder", "templates", "list", "--output", "json")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var templates []coderTemplate
	if err := json.Unmarshal(out, &templates); err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	return templates, nil
}

func pickTemplate(templates []coderTemplate) (string, error) {
	if len(templates) == 1 {
		return templates[0].Template.Name, nil
	}

	// Build lines for fzf
	var lines []string
	for _, t := range templates {
		line := t.Template.Name
		if t.Template.Description != "" {
			line += "  \033[90m" + t.Template.Description + "\033[0m"
		}
		lines = append(lines, line)
	}

	fzfInput := strings.Join(lines, "\n")
	fzf := exec.Command("fzf",
		"--ansi",
		"--reverse",
		"--height=40%",
		"--border=rounded",
		"--prompt=template> ",
		"--header=select a Coder template",
		"--pointer=▶",
		"--no-info",
	)
	fzf.Stdin = strings.NewReader(fzfInput)
	fzf.Stderr = os.Stderr
	out, err := fzf.Output()
	if err != nil {
		return "", nil // user cancelled
	}

	selected := strings.TrimSpace(string(out))
	// Extract just the name (before any description)
	if idx := strings.Index(selected, "  "); idx != -1 {
		selected = selected[:idx]
	}
	return selected, nil
}

// coderWorkspace represents a workspace from `coder list --output json`.
type coderWorkspace struct {
	Name        string `json:"name"`
	LatestBuild struct {
		Status string `json:"status"`
	} `json:"latest_build"`
}

func fetchCoderStatuses() map[string]string {
	cmd := exec.Command("coder", "list", "--output", "json")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var workspaces []coderWorkspace
	if err := json.Unmarshal(out, &workspaces); err != nil {
		return nil
	}
	statuses := make(map[string]string, len(workspaces))
	for _, ws := range workspaces {
		statuses[ws.Name] = ws.LatestBuild.Status
	}
	return statuses
}

var remoteRegisterCmd = &cobra.Command{
	Use:   "register <name>",
	Short: "Register an existing Coder workspace as a remote host",
	Long: `Register a Coder workspace that was already created (e.g. via coder create).
Configures SSH access and detects the workspace's working directory.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		registry, err := core.LoadRegistry(core.DefaultRemotesPath())
		if err != nil {
			return err
		}

		if _, exists := registry.Hosts[name]; exists {
			return fmt.Errorf("remote %q already registered", name)
		}

		// Verify workspace exists via coder list
		fmt.Println("Verifying workspace exists...")
		statuses := fetchCoderStatuses()
		if statuses == nil {
			return fmt.Errorf("could not list Coder workspaces (is coder CLI configured?)")
		}
		wsStatus, exists := statuses[name]
		if !exists {
			return fmt.Errorf("workspace %q not found in Coder (check: coder list)", name)
		}
		fmt.Printf("  Found workspace %q (status: %s)\n", name, wsStatus)

		// Configure SSH access
		fmt.Println("Configuring SSH...")
		configSSH := exec.Command("coder", "config-ssh", "-y")
		configSSH.Stdout = os.Stdout
		configSSH.Stderr = os.Stderr
		if err := configSSH.Run(); err != nil {
			return fmt.Errorf("coder config-ssh: %w", err)
		}

		sshHost := name + ".coder"

		// Detect working directory
		fmt.Println("Detecting workspace directory...")
		pwdCmd := exec.Command("coder", "ssh", name, "pwd")
		pwdOut, err := pwdCmd.Output()
		if err != nil {
			return fmt.Errorf("detect work dir: %w (is the workspace running?)", err)
		}
		workDir := strings.TrimSpace(string(pwdOut))
		if workDir == "" {
			workDir = "/home/coder"
		}

		// Try to detect the template name
		templateName := detectTemplate(name)

		host := &core.RemoteHost{
			Name:     name,
			SSHHost:  sshHost,
			WorkDir:  workDir,
			Template: templateName,
			Created:  time.Now(),
		}
		if err := registry.Add(host); err != nil {
			return err
		}
		if err := registry.Save(); err != nil {
			return err
		}

		fmt.Printf("\nRegistered remote %q\n", name)
		fmt.Printf("  SSH Host:  %s\n", sshHost)
		fmt.Printf("  Work Dir:  %s\n", workDir)
		if templateName != "" {
			fmt.Printf("  Template:  %s\n", templateName)
		}
		fmt.Println("\nCreate a workstream with: ws add <name> --on", name)
		return nil
	},
}

func detectTemplate(workspaceName string) string {
	cmd := exec.Command("coder", "list", "--output", "json")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	var workspaces []struct {
		Name         string `json:"name"`
		TemplateName string `json:"template_name"`
	}
	if err := json.Unmarshal(out, &workspaces); err != nil {
		return ""
	}
	for _, ws := range workspaces {
		if ws.Name == workspaceName {
			return ws.TemplateName
		}
	}
	return ""
}

func init() {
	remoteRemoveCmd.Flags().Bool("delete", false, "Also delete the Coder workspace")
	remoteCmd.AddCommand(remoteAddCmd)
	remoteCmd.AddCommand(remoteRegisterCmd)
	remoteCmd.AddCommand(remoteListCmd)
	remoteCmd.AddCommand(remoteRemoveCmd)
	rootCmd.AddCommand(remoteCmd)
}
