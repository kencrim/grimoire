package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/kencrim/grimoire/libs/core"
)

// removeWorktree removes the git worktree for a local workstream node.
// Uses node.RepoDir to locate the repo; falls back to running from the
// worktree directory itself for backward compat with older state entries.
func removeWorktree(node *core.Node) error {
	if node.Type != core.NodeTypeLocal || node.WorkDir == "" {
		return nil
	}

	repoDir := node.RepoDir
	if repoDir == "" {
		// Backward compat: old nodes don't have RepoDir.
		// Run git from the worktree dir if it still exists.
		if _, err := os.Stat(node.WorkDir); err == nil {
			repoDir = node.WorkDir
		} else {
			return nil // worktree already gone, nothing to remove
		}
	}

	gitRemove := exec.Command("git", "worktree", "remove", node.WorkDir, "--force")
	gitRemove.Dir = repoDir
	if out, err := gitRemove.CombinedOutput(); err != nil {
		return fmt.Errorf("could not remove worktree %s: %s", node.WorkDir, strings.TrimSpace(string(out)))
	}
	return nil
}
