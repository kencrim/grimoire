package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Focus records an active branch-borrow: a workstream's branch temporarily
// checked out in its primary repo (RepoDir) while the workstream's own worktree
// is parked on a detached HEAD. One focus may be active per RepoDir.
type Focus struct {
	RepoDir        string    `json:"repo_dir"`         // main repo receiving the borrowed branch
	WorkstreamID   string    `json:"workstream_id"`    // node that lent its branch
	BorrowedBranch string    `json:"borrowed_branch"`  // branch now checked out in RepoDir (== node.Branch)
	RepoPrevBranch string    `json:"repo_prev_branch"` // branch RepoDir was on before focus (restored on unfocus)
	WorktreePath   string    `json:"worktree_path"`    // lender worktree, now detached
	StashRef       string    `json:"stash_ref,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// FocusRegistry manages the set of active focus sessions, keyed by RepoDir.
type FocusRegistry struct {
	Active map[string]*Focus `json:"active"`
	path   string
}

// DefaultFocusPath returns ~/.config/ws/focus.json.
func DefaultFocusPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ws", "focus.json")
}

// LoadFocusRegistry reads the focus registry from disk, or returns an empty one.
func LoadFocusRegistry(path string) (*FocusRegistry, error) {
	r := &FocusRegistry{
		Active: make(map[string]*Focus),
		path:   path,
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return r, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read focus: %w", err)
	}

	if err := json.Unmarshal(data, r); err != nil {
		return nil, fmt.Errorf("parse focus: %w", err)
	}
	if r.Active == nil {
		r.Active = make(map[string]*Focus)
	}
	r.path = path
	return r, nil
}

// Save writes the registry to disk.
func (r *FocusRegistry) Save() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.path, data, 0o644)
}

// Set records (or replaces) the focus session for its RepoDir.
func (r *FocusRegistry) Set(f *Focus) {
	r.Active[f.RepoDir] = f
}

// ForRepo returns the active focus session for a repo dir, if any.
func (r *FocusRegistry) ForRepo(repoDir string) (*Focus, bool) {
	f, ok := r.Active[repoDir]
	return f, ok
}

// ForWorkstream returns the active focus session lent by a workstream, if any.
func (r *FocusRegistry) ForWorkstream(id string) (*Focus, bool) {
	for _, f := range r.Active {
		if f.WorkstreamID == id {
			return f, true
		}
	}
	return nil, false
}

// Clear removes the focus session for a repo dir.
func (r *FocusRegistry) Clear(repoDir string) {
	delete(r.Active, repoDir)
}
