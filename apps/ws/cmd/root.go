package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ws",
	Short: "Workstream manager — orchestrate agents across worktrees",
	Long: `ws manages a DAG of workstreams, each backed by a git worktree
(local) or a remote devpod. Each workstream gets its own tmux session
with agents running inside it.

  ws add auth --agent claude       Create a workstream
  ws add auth/oauth --agent amp    Nest under an existing workstream
  ws list                          Show the workstream tree
  ws switch auth                   Switch to a workstream's tmux session
  ws kill auth                     Tear down a workstream and its children
  ws status                        Show status of all workstreams`,
}

func Execute() error {
	return rootCmd.Execute()
}
