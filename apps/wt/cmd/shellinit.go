package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var shellInitCmd = &cobra.Command{
	Use:   "shell-init",
	Short: "Print shell functions for wt integration",
	Long:  `Add to your .zshrc: eval "$(wt shell-init)"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Print(`# wt — git worktree quick-nav
function wt() {
  local dir
  dir=$(command wt "$@") && [ -n "$dir" ] && cd "$dir"
}
`)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(shellInitCmd)
}
