package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// worktreeEntry represents a parsed git worktree from porcelain output.
type worktreeEntry struct {
	Path   string
	HEAD   string
	Branch string
	Bare   bool
}

var rootCmd = &cobra.Command{
	Use:   "wt",
	Short: "Quick-nav for git worktrees",
	Long: `wt lists git worktrees in the current repo and prints the selected path.

Wrap in a shell function for cd support:
  eval "$(wt shell-init)"

Then just type wt to pick a worktree and cd into it.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := listGitWorktrees()
		if err != nil {
			return err
		}

		if len(entries) == 0 {
			fmt.Fprintln(os.Stderr, "No worktrees found.")
			return nil
		}

		lines := formatWorktreeLines(entries)
		fzfInput := strings.Join(lines, "\n")

		fzf := exec.Command("fzf",
			"--ansi",
			"--reverse",
			"--height=40%",
			"--border=rounded",
			"--prompt=worktree> ",
			"--header=cd to a worktree",
			"--pointer=▶",
			"--no-info",
		)
		fzf.Stdin = strings.NewReader(fzfInput)
		fzf.Stderr = os.Stderr
		out, err := fzf.Output()
		if err != nil {
			// User pressed Escape or Ctrl-C
			return nil
		}

		selected := strings.TrimSpace(string(out))
		if selected == "" {
			return nil
		}

		absPath := extractPathFromLine(selected, entries)
		if absPath == "" {
			return fmt.Errorf("could not parse selection")
		}

		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "directory no longer exists: %s\n", absPath)
			return fmt.Errorf("directory does not exist: %s", absPath)
		}

		fmt.Print(absPath)
		return nil
	},
}

// listGitWorktrees runs `git worktree list --porcelain` and parses the output.
func listGitWorktrees() ([]worktreeEntry, error) {
	gitCmd := exec.Command("git", "worktree", "list", "--porcelain")
	out, err := gitCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("not a git repository (or git not installed)")
	}

	var entries []worktreeEntry
	var current worktreeEntry

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "worktree "):
			if current.Path != "" {
				entries = append(entries, current)
			}
			current = worktreeEntry{Path: strings.TrimPrefix(line, "worktree ")}
		case strings.HasPrefix(line, "HEAD "):
			current.HEAD = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimPrefix(line, "branch ")
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		case line == "bare":
			current.Bare = true
		}
	}
	if current.Path != "" {
		entries = append(entries, current)
	}

	return entries, nil
}

// formatWorktreeLines builds ANSI-colored display lines for fzf.
func formatWorktreeLines(entries []worktreeEntry) []string {
	lines := make([]string, 0, len(entries))

	for _, e := range entries {
		dir := filepath.Base(e.Path)
		branch := e.Branch
		if branch == "" && e.Bare {
			branch = "(bare)"
		} else if branch == "" {
			branch = e.HEAD[:8] // detached HEAD — show short hash
		}

		line := fmt.Sprintf("%-30s \033[36m%s\033[0m", dir, branch)
		lines = append(lines, line)
	}
	return lines
}

// extractPathFromLine maps a selected fzf line back to the absolute worktree path.
func extractPathFromLine(selected string, entries []worktreeEntry) string {
	cleaned := stripAnsi(selected)
	dirName := strings.TrimSpace(strings.Fields(cleaned)[0])

	for _, e := range entries {
		if filepath.Base(e.Path) == dirName {
			return e.Path
		}
	}
	return ""
}

// stripAnsi removes ANSI escape sequences from a string.
func stripAnsi(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			i = j + 1
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
}

func Execute() error {
	return rootCmd.Execute()
}
