package core

import (
	"path/filepath"
	"testing"
	"time"
)

func freshFocusRegistry(t *testing.T) *FocusRegistry {
	t.Helper()
	return &FocusRegistry{
		Active: make(map[string]*Focus),
		path:   filepath.Join(t.TempDir(), "focus.json"),
	}
}

func TestFocusRegistry_SetForRepoClear(t *testing.T) {
	r := freshFocusRegistry(t)

	f := &Focus{
		RepoDir:        "/repo",
		WorkstreamID:   "foo",
		BorrowedBranch: "ws-foo",
		RepoPrevBranch: "main",
		WorktreePath:   "/repo-foo",
		CreatedAt:      time.Now(),
	}
	r.Set(f)

	got, ok := r.ForRepo("/repo")
	if !ok {
		t.Fatal("expected focus after Set")
	}
	if got.WorkstreamID != "foo" {
		t.Errorf("WorkstreamID = %q, want %q", got.WorkstreamID, "foo")
	}

	byWs, ok := r.ForWorkstream("foo")
	if !ok || byWs.RepoDir != "/repo" {
		t.Fatalf("ForWorkstream lookup failed: ok=%v", ok)
	}

	r.Clear("/repo")
	if _, ok := r.ForRepo("/repo"); ok {
		t.Fatal("expected focus gone after Clear")
	}
}

func TestFocusRegistry_SaveLoad(t *testing.T) {
	r := freshFocusRegistry(t)
	r.Set(&Focus{
		RepoDir:        "/home/user/grimoire",
		WorkstreamID:   "auth",
		BorrowedBranch: "ws-auth",
		RepoPrevBranch: "main",
		WorktreePath:   "/home/user/grimoire-auth",
		CreatedAt:      time.Now(),
	})
	if err := r.Save(); err != nil {
		t.Fatalf("unexpected error saving: %v", err)
	}

	loaded, err := LoadFocusRegistry(r.path)
	if err != nil {
		t.Fatalf("unexpected error loading: %v", err)
	}
	if len(loaded.Active) != 1 {
		t.Fatalf("expected 1 active focus, got %d", len(loaded.Active))
	}
	got, ok := loaded.ForRepo("/home/user/grimoire")
	if !ok {
		t.Fatal("expected to find focus after load")
	}
	if got.BorrowedBranch != "ws-auth" {
		t.Errorf("BorrowedBranch = %q, want %q", got.BorrowedBranch, "ws-auth")
	}
}

func TestFocusRegistry_LoadNonExistent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does_not_exist", "focus.json")

	r, err := LoadFocusRegistry(path)
	if err != nil {
		t.Fatalf("expected no error for nonexistent file, got: %v", err)
	}
	if r == nil || len(r.Active) != 0 {
		t.Fatal("expected empty registry")
	}
}
