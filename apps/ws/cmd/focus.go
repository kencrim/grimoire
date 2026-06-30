package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/kencrim/grimoire/libs/core"
	"github.com/spf13/cobra"
)

var focusAutostash bool
var unfocusForce bool

var focusCmd = &cobra.Command{
	Use:   "focus [name]",
	Short: "Borrow a workstream's branch into its main repo so you can run it there",
	Long: `Temporarily check out a workstream's branch in its primary repo (RepoDir),
where the git-ignored env config and build artifacts live, so you can actually
run it. The workstream's own worktree is parked on a detached HEAD while focused.

Run with no name to pick a workstream interactively. Restore everything with
'ws unfocus'.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tree, err := core.LoadTree(core.DefaultStatePath())
		if err != nil {
			return err
		}

		var name string
		if len(args) == 1 {
			name = args[0]
		} else {
			name, err = pickLocalWorkstream(tree)
			if err != nil {
				return err
			}
			if name == "" {
				return nil // user cancelled
			}
		}

		return focusWorkstream(tree, name)
	},
}

var unfocusCmd = &cobra.Command{
	Use:   "unfocus [name]",
	Short: "Restore a focused workstream's repo and worktree to their branches",
	Long: `Undo 'ws focus': return the main repo to the branch it was on and
re-attach the workstream's worktree to its own branch.

Run with no name when exactly one focus is active.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		reg, err := core.LoadFocusRegistry(core.DefaultFocusPath())
		if err != nil {
			return err
		}

		var f *core.Focus
		if len(args) == 1 {
			var ok bool
			f, ok = reg.ForWorkstream(args[0])
			if !ok {
				return fmt.Errorf("workstream %q is not focused", args[0])
			}
		} else {
			switch len(reg.Active) {
			case 0:
				return fmt.Errorf("no active focus")
			case 1:
				for _, only := range reg.Active {
					f = only
				}
			default:
				var ids []string
				for _, a := range reg.Active {
					ids = append(ids, a.WorkstreamID)
				}
				sort.Strings(ids)
				return fmt.Errorf("multiple focuses active; specify one:\n  %s",
					strings.Join(ids, "\n  "))
			}
		}

		return unfocusWorkstream(reg, f)
	},
}

func focusWorkstream(tree *core.Tree, name string) error {
	node, ok := tree.Nodes[name]
	if !ok {
		return fmt.Errorf("workstream %q not found", name)
	}
	if node.Type != core.NodeTypeLocal {
		return fmt.Errorf("can only focus local workstreams; %q is %s", name, node.Type)
	}
	if node.RepoDir == "" {
		return fmt.Errorf("workstream %q has no recorded repo dir; cannot focus", name)
	}
	if node.WorkDir == "" {
		return fmt.Errorf("workstream %q has no worktree; cannot focus", name)
	}

	repoDir := node.RepoDir
	worktree := node.WorkDir
	branch := node.Branch

	reg, err := core.LoadFocusRegistry(core.DefaultFocusPath())
	if err != nil {
		return err
	}

	// One focus per repo: if something else is focused here, unfocus it first.
	if existing, ok := reg.ForRepo(repoDir); ok {
		if existing.WorkstreamID == name {
			fmt.Printf("Workstream %q is already focused into %s\n", name, repoDir)
			return nil
		}
		fmt.Printf("Unfocusing %q first (one focus per repo)...\n", existing.WorkstreamID)
		if err := unfocusWorkstream(reg, existing); err != nil {
			return fmt.Errorf("could not unfocus current workstream %q: %w", existing.WorkstreamID, err)
		}
	}

	prevBranch, err := gitCurrentBranch(repoDir)
	if err != nil {
		return fmt.Errorf("read current branch of %s: %w", repoDir, err)
	}
	if prevBranch == "" {
		return fmt.Errorf("%s is in detached HEAD; refusing to focus", repoDir)
	}
	if prevBranch == branch {
		return fmt.Errorf("%s is already on branch %q", repoDir, branch)
	}

	// The receiving repo must be clean (ignored env files are untouched by switch
	// and don't count). Tracked changes would otherwise be carried or block.
	dirty, err := gitDirty(repoDir)
	if err != nil {
		return err
	}
	var stashRef string
	if dirty {
		if !focusAutostash {
			return fmt.Errorf("%s has uncommitted changes; commit them or re-run with --autostash", repoDir)
		}
		stashRef = "ws-focus:" + name
		if out, err := runGit(repoDir, "stash", "push", "-u", "-m", stashRef); err != nil {
			return fmt.Errorf("git stash: %s", out)
		}
	}

	// 1. Free the branch: detach the lender worktree at the same commit (no
	//    working-tree change, uncommitted changes in the lender are preserved).
	if out, err := runGit(worktree, "switch", "--detach"); err != nil {
		if stashRef != "" {
			popStash(repoDir, stashRef)
		}
		return fmt.Errorf("detach worktree %s: %s", worktree, out)
	}

	// 2. Borrow the branch into the main repo. Git-ignored env files stay put.
	if out, err := runGit(repoDir, "switch", branch); err != nil {
		// Roll back the detach.
		runGit(worktree, "switch", branch)
		if stashRef != "" {
			popStash(repoDir, stashRef)
		}
		return fmt.Errorf("switch %s to %s: %s", repoDir, branch, out)
	}

	reg.Set(&core.Focus{
		RepoDir:        repoDir,
		WorkstreamID:   name,
		BorrowedBranch: branch,
		RepoPrevBranch: prevBranch,
		WorktreePath:   worktree,
		StashRef:       stashRef,
		CreatedAt:      time.Now(),
	})
	if err := reg.Save(); err != nil {
		return err
	}

	fmt.Printf("Focused %q into %s\n", name, repoDir)
	fmt.Printf("  Branch:   %s (was %s)\n", branch, prevBranch)
	fmt.Printf("  Parked:   %s (detached)\n", worktree)
	if stashRef != "" {
		fmt.Printf("  Stashed:  uncommitted changes in %s (restored on unfocus)\n", repoDir)
	}
	fmt.Printf("\n  cd %s && run it\n", repoDir)
	return nil
}

func unfocusWorkstream(reg *core.FocusRegistry, f *core.Focus) error {
	// Validate the live state still matches what focus recorded.
	if !unfocusForce {
		cur, err := gitCurrentBranch(f.RepoDir)
		if err != nil {
			return err
		}
		if cur != f.BorrowedBranch {
			return fmt.Errorf("%s is on %q, expected %q; re-run with --force to override",
				f.RepoDir, branchOrDetached(cur), f.BorrowedBranch)
		}
		wcur, err := gitCurrentBranch(f.WorktreePath)
		if err != nil {
			return err
		}
		if wcur != "" {
			return fmt.Errorf("worktree %s is on branch %q, expected detached; re-run with --force to override",
				f.WorktreePath, wcur)
		}
	}

	// 1. Return the branch: restore the main repo to its previous branch.
	if out, err := runGit(f.RepoDir, "switch", f.RepoPrevBranch); err != nil {
		return fmt.Errorf("restore %s to %s: %s", f.RepoDir, f.RepoPrevBranch, out)
	}

	// 2. Re-attach the lender worktree to its own branch.
	if out, err := runGit(f.WorktreePath, "switch", f.BorrowedBranch); err != nil {
		fmt.Printf("  Warning: could not re-attach %s to %s: %s\n", f.WorktreePath, f.BorrowedBranch, out)
	}

	// 3. Restore any stashed changes in the main repo.
	if f.StashRef != "" {
		if err := popStash(f.RepoDir, f.StashRef); err != nil {
			fmt.Printf("  Warning: could not restore stash %q: %v\n", f.StashRef, err)
		}
	}

	reg.Clear(f.RepoDir)
	if err := reg.Save(); err != nil {
		return err
	}

	fmt.Printf("Unfocused %q\n", f.WorkstreamID)
	fmt.Printf("  %s restored to %s\n", f.RepoDir, f.RepoPrevBranch)
	fmt.Printf("  %s re-attached to %s\n", f.WorktreePath, f.BorrowedBranch)
	return nil
}

// pickLocalWorkstream shows an fzf picker of local workstreams and returns the
// selected ID, or "" if the user cancelled.
func pickLocalWorkstream(tree *core.Tree) (string, error) {
	var ids []string
	for id, n := range tree.Nodes {
		if n.Type == core.NodeTypeLocal {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return "", fmt.Errorf("no local workstreams to focus")
	}
	sort.Strings(ids)

	var lines []string
	for _, id := range ids {
		lines = append(lines, fmt.Sprintf("%s\t%s", id, tree.Nodes[id].Branch))
	}

	fzf := exec.Command("fzf",
		"--ansi",
		"--reverse",
		"--height=40%",
		"--border=rounded",
		"--prompt=focus> ",
		"--header=borrow a workstream's branch into its main repo",
		"--pointer=▶",
		"--with-nth=1",
		"--delimiter=\t",
		"--no-info",
	)
	fzf.Stdin = strings.NewReader(strings.Join(lines, "\n"))
	fzf.Stderr = os.Stderr
	out, err := fzf.Output()
	if err != nil {
		return "", nil // cancelled
	}
	selected := strings.TrimSpace(string(out))
	if selected == "" {
		return "", nil
	}
	return strings.SplitN(selected, "\t", 2)[0], nil
}

// gitCurrentBranch returns the checked-out branch of a worktree, or "" if HEAD
// is detached.
func gitCurrentBranch(dir string) (string, error) {
	out, err := runGit(dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		// Detached HEAD: symbolic-ref exits non-zero with no output.
		if strings.TrimSpace(out) == "" {
			return "", nil
		}
		return "", fmt.Errorf("git symbolic-ref: %s", out)
	}
	return strings.TrimSpace(out), nil
}

// gitDirty reports whether a worktree has uncommitted changes to tracked files.
// Ignored files (env config, build artifacts) do not count.
func gitDirty(dir string) (bool, error) {
	out, err := runGit(dir, "status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return false, fmt.Errorf("git status: %s", out)
	}
	return strings.TrimSpace(out) != "", nil
}

// popStash finds the stash created with the given message and pops it.
func popStash(dir, message string) error {
	out, err := runGit(dir, "stash", "list")
	if err != nil {
		return fmt.Errorf("git stash list: %s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, message) {
			ref := strings.SplitN(line, ":", 2)[0]
			if popOut, err := runGit(dir, "stash", "pop", ref); err != nil {
				return fmt.Errorf("git stash pop: %s", popOut)
			}
			return nil
		}
	}
	return nil // nothing matching; treat as already restored
}

func branchOrDetached(b string) string {
	if b == "" {
		return "(detached)"
	}
	return b
}

// runGit runs a git command in dir and returns combined output.
func runGit(dir string, args ...string) (string, error) {
	c := exec.Command("git", args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	return string(out), err
}

func init() {
	focusCmd.Flags().BoolVar(&focusAutostash, "autostash", false, "Stash uncommitted changes in the main repo, restore them on unfocus")
	unfocusCmd.Flags().BoolVar(&unfocusForce, "force", false, "Unfocus even if the live branch state has drifted from what focus recorded")
	rootCmd.AddCommand(focusCmd)
	rootCmd.AddCommand(unfocusCmd)
}
