package cmd

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/kencrim/grimoire/libs/core"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ws",
	Short: "Workstream manager — orchestrate agents across worktrees",
	Long: `ws manages a DAG of workstreams, each backed by a git worktree
(local) or a remote devpod. Each workstream gets its own tmux session
with agents running inside it.

  ws add auth                       Create a workstream (uses Claude Code)
  ws add auth/oauth --agent amp    Nest under an existing workstream
  ws list                          Show the workstream tree
  ws switch auth                   Switch to a workstream's tmux session
  ws kill auth                     Tear down a workstream and its children
  ws status                        Show status of all workstreams

Running ws with no subcommand opens an interactive workstream picker.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		tree, err := core.LoadTree(core.DefaultStatePath())
		if err != nil {
			return err
		}

		if len(tree.Nodes) == 0 {
			fmt.Println("No workstreams. Create one with: ws add <name> --agent claude")
			return nil
		}

		// Build tree-formatted lines for fzf
		lines := buildTreeLines(tree)
		if len(lines) == 0 {
			fmt.Println("No workstreams.")
			return nil
		}

		// Pipe to fzf
		fzfInput := strings.Join(lines, "\n")
		fzf := exec.Command("fzf",
			"--ansi",
			"--reverse",
			"--height=40%",
			"--border=rounded",
			"--prompt=workstream> ",
			"--header=switch to a workstream",
			"--pointer=▶",
			"--no-info",
		)
		fzf.Stdin = strings.NewReader(fzfInput)
		fzf.Stderr = os.Stderr
		out, err := fzf.Output()
		if err != nil {
			// User pressed Escape or Ctrl+C
			return nil
		}

		selected := strings.TrimSpace(string(out))
		if selected == "" {
			return nil
		}

		// Extract the workstream ID from the selected line
		// Lines look like: "  ├── auth [claude] (matrix)"
		// or:              "auth [claude] (matrix)"
		// The ID is the first word after any tree decoration
		wsID := extractIDFromLine(selected, tree)
		if wsID == "" {
			return fmt.Errorf("could not parse selection: %s", selected)
		}

		node, exists := tree.Nodes[wsID]
		if !exists {
			return fmt.Errorf("workstream %q not found", wsID)
		}

		// Apply shader
		if node.Shader != "" {
			if err := applyShader(node.Shader); err != nil {
				log.Printf("[ws] warning: could not apply shader: %v", err)
			}
		}

		// Switch to the workstream
		if os.Getenv("TMUX") != "" {
			tmux := exec.Command("tmux", "switch-client", "-t", node.Session)
			tmux.Stdin = os.Stdin
			tmux.Stdout = os.Stdout
			tmux.Stderr = os.Stderr
			return tmux.Run()
		}

		tmux := exec.Command("tmux", "attach-session", "-t", node.Session)
		tmux.Stdin = os.Stdin
		tmux.Stdout = os.Stdout
		tmux.Stderr = os.Stderr
		return tmux.Run()
	},
}

// buildTreeLines generates indented tree lines for fzf display.
func buildTreeLines(tree *core.Tree) []string {
	var lines []string
	roots := tree.Roots()
	sort.Slice(roots, func(i, j int) bool {
		return roots[i].ID < roots[j].ID
	})

	for i, root := range roots {
		last := i == len(roots)-1
		appendTreeNode(tree, root, "", last, &lines)
	}
	return lines
}

func appendTreeNode(tree *core.Tree, node *core.Node, prefix string, last bool, lines *[]string) {
	connector := "├── "
	if last {
		connector = "└── "
	}
	if prefix == "" {
		connector = ""
	}

	// Color the theme label using ANSI based on the border color
	themeLabel := ""
	if node.Shader != "" {
		label := strings.TrimSuffix(node.Shader, ".glsl")
		label = strings.ReplaceAll(label, "-", " ")
		themeLabel = fmt.Sprintf(" \033[90m(%s)\033[0m", label)
	}

	agent := ""
	if node.Agent != "" {
		agent = fmt.Sprintf(" \033[36m[%s]\033[0m", node.Agent)
	}

	// Color the workstream name using the border color.
	// If the node's color is too dark (old tint value), look up the parent's color.
	displayColor := node.Color
	if node.ParentID != "" && !isBrightEnough(displayColor) {
		if parent, ok := tree.Nodes[node.ParentID]; ok {
			displayColor = parent.Color
		}
	}
	// Last resort: resolve from shader or theme
	if !isBrightEnough(displayColor) {
		if node.Shader != "" {
			displayColor = core.ThemeByShader(node.Shader).Border
		} else {
			displayColor = core.WorkstreamThemes[0].Border
		}
	}
	coloredName := colorize(node.ID, displayColor)

	line := fmt.Sprintf("%s%s%s%s%s", prefix, connector, coloredName, agent, themeLabel)
	*lines = append(*lines, line)

	children := tree.Children(node.ID)
	sort.Slice(children, func(i, j int) bool {
		return children[i].ID < children[j].ID
	})

	childPrefix := prefix
	if prefix != "" || len(children) > 0 {
		if last {
			childPrefix += "    "
		} else {
			childPrefix += "│   "
		}
	}

	for i, child := range children {
		appendTreeNode(tree, child, childPrefix, i == len(children)-1, lines)
	}
}

// isBrightEnough checks if a hex color is bright enough to be visible on a dark background.
func isBrightEnough(hexColor string) bool {
	if hexColor == "" {
		return false
	}
	hex := strings.TrimPrefix(hexColor, "#")
	if len(hex) != 6 {
		return false
	}
	var r, g, b int
	fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	// Perceived brightness
	brightness := (r*299 + g*587 + b*114) / 1000
	return brightness > 80
}

// colorize wraps text in ANSI color based on a hex color string.
func colorize(text, hexColor string) string {
	if hexColor == "" {
		return text
	}
	// Parse hex color to RGB
	hex := strings.TrimPrefix(hexColor, "#")
	if len(hex) != 6 {
		return text
	}
	var r, g, b int
	fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	return fmt.Sprintf("\033[38;2;%d;%d;%dm%s\033[0m", r, g, b, text)
}

// extractIDFromLine parses a workstream ID from a tree-formatted fzf line.
func extractIDFromLine(line string, tree *core.Tree) string {
	// Strip ANSI escape codes
	cleaned := stripAnsi(line)
	// Remove tree decoration
	cleaned = strings.TrimLeft(cleaned, " │├└──")
	cleaned = strings.TrimSpace(cleaned)

	// The ID is everything before the first " [" (agent bracket)
	if idx := strings.Index(cleaned, " ["); idx != -1 {
		cleaned = cleaned[:idx]
	}
	// Or before " (" (theme label)
	if idx := strings.Index(cleaned, " ("); idx != -1 {
		cleaned = cleaned[:idx]
	}

	cleaned = strings.TrimSpace(cleaned)

	// Verify it's a real workstream ID
	if _, exists := tree.Nodes[cleaned]; exists {
		return cleaned
	}

	// Fallback: try matching against all node IDs
	for id := range tree.Nodes {
		if strings.Contains(cleaned, id) {
			return id
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
			// Skip until 'm'
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
