package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Repo represents a registered local git repository.
type Repo struct {
	Name    string    `json:"name"`       // user-chosen alias, e.g. "grimoire"
	Path    string    `json:"path"`       // absolute path to git repo root
	Created time.Time `json:"created_at"`
}

// RepoRegistry manages the set of registered local repositories.
type RepoRegistry struct {
	Repos map[string]*Repo `json:"repos"`
	path  string
}

// DefaultReposPath returns ~/.config/ws/repos.json.
func DefaultReposPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ws", "repos.json")
}

// LoadRepoRegistry reads the repo registry from disk, or returns an empty registry.
func LoadRepoRegistry(path string) (*RepoRegistry, error) {
	r := &RepoRegistry{
		Repos: make(map[string]*Repo),
		path:  path,
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return r, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read repos: %w", err)
	}

	if err := json.Unmarshal(data, r); err != nil {
		return nil, fmt.Errorf("parse repos: %w", err)
	}
	r.path = path
	return r, nil
}

// Save writes the registry to disk.
func (r *RepoRegistry) Save() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.path, data, 0o644)
}

// Add registers a new local repository.
func (r *RepoRegistry) Add(repo *Repo) error {
	if _, exists := r.Repos[repo.Name]; exists {
		return fmt.Errorf("repo %q already exists", repo.Name)
	}
	r.Repos[repo.Name] = repo
	return nil
}

// Get returns a repo by name.
func (r *RepoRegistry) Get(name string) (*Repo, bool) {
	repo, ok := r.Repos[name]
	return repo, ok
}

// Remove deletes a repo from the registry.
func (r *RepoRegistry) Remove(name string) error {
	if _, exists := r.Repos[name]; !exists {
		return fmt.Errorf("repo %q not found", name)
	}
	delete(r.Repos, name)
	return nil
}
