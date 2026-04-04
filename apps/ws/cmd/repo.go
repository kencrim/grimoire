package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kencrim/grimoire/libs/core"
	"github.com/spf13/cobra"
)

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Manage known local git repositories",
	Long: `Register, list, and remove local git repos so workstreams can be
created from anywhere (including the mobile companion app).

  ws repo add grimoire ~/GitHub/grimoire   Register a repo
  ws repo add grimoire                     Register cwd as a repo
  ws repo list                             Show registered repos
  ws repo remove grimoire                  Unregister a repo`,
}

var repoAddCmd = &cobra.Command{
	Use:   "add <name> [path]",
	Short: "Register a local git repository",
	Long: `Register a local git repository by alias. If path is omitted, the
current directory is used. The path must be a git repository root.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		var repoPath string
		if len(args) >= 2 {
			repoPath = args[1]
		} else {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}
			repoPath = cwd
		}

		// Resolve to absolute path
		absPath, err := filepath.Abs(repoPath)
		if err != nil {
			return fmt.Errorf("resolve path: %w", err)
		}

		// Validate it's a git repo by checking git rev-parse --show-toplevel
		gitTopLevel := exec.Command("git", "rev-parse", "--show-toplevel")
		gitTopLevel.Dir = absPath
		out, err := gitTopLevel.Output()
		if err != nil {
			return fmt.Errorf("%q is not a git repository", absPath)
		}
		repoRoot := strings.TrimSpace(string(out))

		// Warn if the provided path isn't the repo root
		if repoRoot != absPath {
			fmt.Printf("Note: using repo root %s (not %s)\n", repoRoot, absPath)
		}

		registry, err := core.LoadRepoRegistry(core.DefaultReposPath())
		if err != nil {
			return err
		}

		repo := &core.Repo{
			Name:    name,
			Path:    repoRoot,
			Created: time.Now(),
		}
		if err := registry.Add(repo); err != nil {
			return err
		}
		if err := registry.Save(); err != nil {
			return err
		}

		fmt.Printf("Registered repo %q\n", name)
		fmt.Printf("  Path: %s\n", repoRoot)
		return nil
	},
}

var repoListCmd = &cobra.Command{
	Use:     "list",
	Short:   "Show registered repositories",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		registry, err := core.LoadRepoRegistry(core.DefaultReposPath())
		if err != nil {
			return err
		}

		if len(registry.Repos) == 0 {
			fmt.Println("No repos registered. Add one with: ws repo add <name> [path]")
			return nil
		}

		fmt.Printf("%-20s %s\n", "NAME", "PATH")
		fmt.Printf("%-20s %s\n", "----", "----")
		for _, repo := range registry.Repos {
			fmt.Printf("%-20s %s\n", repo.Name, repo.Path)
		}
		return nil
	},
}

var repoRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Unregister a repository",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		registry, err := core.LoadRepoRegistry(core.DefaultReposPath())
		if err != nil {
			return err
		}

		if err := registry.Remove(name); err != nil {
			return err
		}
		if err := registry.Save(); err != nil {
			return err
		}

		fmt.Printf("Removed repo %q\n", name)
		return nil
	},
}

func init() {
	repoCmd.AddCommand(repoAddCmd)
	repoCmd.AddCommand(repoListCmd)
	repoCmd.AddCommand(repoRemoveCmd)
	rootCmd.AddCommand(repoCmd)
}
