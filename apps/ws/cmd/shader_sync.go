package cmd

import (
	"os/exec"
	"strings"

	"github.com/kencrim/grimoire/libs/core"
	"github.com/spf13/cobra"
)

var shaderSyncCmd = &cobra.Command{
	Use:    "shader-sync",
	Short:  "Sync Ghostty shader to the active tmux session's workstream",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		reset, _ := cmd.Flags().GetBool("reset")
		if reset {
			return applyShader("animated-gradient-shader.glsl")
		}

		// Find the focused tmux client and its session.
		out, err := exec.Command("tmux", "list-clients", "-F", "#{client_flags} #{session_name}").Output()
		if err != nil {
			return applyShader("animated-gradient-shader.glsl")
		}

		var focusedSession string
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if strings.Contains(line, "focused") {
				parts := strings.SplitN(strings.TrimSpace(line), " ", 2)
				if len(parts) == 2 {
					focusedSession = parts[1]
				}
				break
			}
		}

		if focusedSession == "" {
			return applyShader("animated-gradient-shader.glsl")
		}

		tree, err := core.LoadTree(core.DefaultStatePath())
		if err != nil {
			return applyShader("animated-gradient-shader.glsl")
		}

		// Find a node matching this session — could be a root or a child
		for _, node := range tree.Nodes {
			if node.Session != focusedSession {
				continue
			}

			// Try shader field first
			if node.Shader != "" {
				return applyShader(node.Shader)
			}

			// Fallback: resolve from border color
			if node.Color != "" {
				theme := core.ThemeByBorder(node.Color)
				return applyShader(theme.Shader)
			}

			// Has a parent? Inherit from parent
			if node.ParentID != "" {
				if parent, ok := tree.Nodes[node.ParentID]; ok {
					if parent.Shader != "" {
						return applyShader(parent.Shader)
					}
					if parent.Color != "" {
						theme := core.ThemeByBorder(parent.Color)
						return applyShader(theme.Shader)
					}
				}
			}
		}

		return applyShader("animated-gradient-shader.glsl")
	},
}

func init() {
	shaderSyncCmd.Flags().Bool("reset", false, "Reset to default gradient shader")
	rootCmd.AddCommand(shaderSyncCmd)
}
