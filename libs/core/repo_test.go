package core

import (
	"path/filepath"
	"testing"
	"time"
)

func freshRepoRegistry(t *testing.T) *RepoRegistry {
	t.Helper()
	return &RepoRegistry{
		Repos: make(map[string]*Repo),
		path:  filepath.Join(t.TempDir(), "repos.json"),
	}
}

func TestRepoRegistry_AddGetRemove(t *testing.T) {
	r := freshRepoRegistry(t)

	repo := &Repo{Name: "myrepo", Path: "/tmp/myrepo", Created: time.Now()}
	if err := r.Add(repo); err != nil {
		t.Fatalf("unexpected error adding repo: %v", err)
	}

	got, ok := r.Get("myrepo")
	if !ok {
		t.Fatal("expected to find repo after Add")
	}
	if got.Path != "/tmp/myrepo" {
		t.Errorf("Path = %q, want %q", got.Path, "/tmp/myrepo")
	}

	if err := r.Remove("myrepo"); err != nil {
		t.Fatalf("unexpected error removing repo: %v", err)
	}

	_, ok = r.Get("myrepo")
	if ok {
		t.Fatal("expected repo to be gone after Remove")
	}
}

func TestRepoRegistry_SaveLoad(t *testing.T) {
	r := freshRepoRegistry(t)

	repo := &Repo{Name: "grimoire", Path: "/home/user/grimoire", Created: time.Now()}
	if err := r.Add(repo); err != nil {
		t.Fatalf("unexpected error adding repo: %v", err)
	}
	if err := r.Save(); err != nil {
		t.Fatalf("unexpected error saving: %v", err)
	}

	loaded, err := LoadRepoRegistry(r.path)
	if err != nil {
		t.Fatalf("unexpected error loading: %v", err)
	}

	if len(loaded.Repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(loaded.Repos))
	}

	got, ok := loaded.Get("grimoire")
	if !ok {
		t.Fatal("expected to find repo after load")
	}
	if got.Path != "/home/user/grimoire" {
		t.Errorf("Path = %q, want %q", got.Path, "/home/user/grimoire")
	}
}

func TestRepoRegistry_DuplicateAdd(t *testing.T) {
	r := freshRepoRegistry(t)

	repo := &Repo{Name: "dup", Path: "/tmp/dup", Created: time.Now()}
	if err := r.Add(repo); err != nil {
		t.Fatalf("unexpected error on first add: %v", err)
	}

	err := r.Add(&Repo{Name: "dup", Path: "/tmp/other", Created: time.Now()})
	if err == nil {
		t.Fatal("expected error when adding duplicate name, got nil")
	}
}

func TestRepoRegistry_RemoveNotFound(t *testing.T) {
	r := freshRepoRegistry(t)

	err := r.Remove("ghost")
	if err == nil {
		t.Fatal("expected error when removing nonexistent repo, got nil")
	}
}

func TestRepoRegistry_LoadNonExistent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does_not_exist", "repos.json")

	r, err := LoadRepoRegistry(path)
	if err != nil {
		t.Fatalf("expected no error for nonexistent file, got: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil registry")
	}
	if len(r.Repos) != 0 {
		t.Fatalf("expected empty Repos map, got %d entries", len(r.Repos))
	}
}
