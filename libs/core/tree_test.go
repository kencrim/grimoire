package core

import (
	"path/filepath"
	"testing"
	"time"
)

func makeNode(id, parentID string) *Node {
	return &Node{
		ID: id, Name: id, Branch: "ws-" + id,
		ParentID: parentID, Type: NodeTypeLocal,
		Status: StatusRunning, WorkDir: "/tmp/" + id,
		Session: "ws/" + id, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
}

func freshTree(t *testing.T) *Tree {
	t.Helper()
	return &Tree{
		Nodes: make(map[string]*Node),
		path:  filepath.Join(t.TempDir(), "state.json"),
	}
}

func TestAdd_Root(t *testing.T) {
	tr := freshTree(t)
	n := makeNode("root1", "")

	if err := tr.Add(n); err != nil {
		t.Fatalf("unexpected error adding root node: %v", err)
	}

	if _, ok := tr.Nodes["root1"]; !ok {
		t.Fatal("root node not found in Nodes map after Add")
	}
}

func TestAdd_Child(t *testing.T) {
	tr := freshTree(t)
	parent := makeNode("parent", "")
	child := makeNode("child", "parent")

	if err := tr.Add(parent); err != nil {
		t.Fatalf("unexpected error adding parent: %v", err)
	}
	if err := tr.Add(child); err != nil {
		t.Fatalf("unexpected error adding child: %v", err)
	}

	if _, ok := tr.Nodes["parent"]; !ok {
		t.Fatal("parent node not found in Nodes map")
	}
	if _, ok := tr.Nodes["child"]; !ok {
		t.Fatal("child node not found in Nodes map")
	}
}

func TestAdd_DuplicateID(t *testing.T) {
	tr := freshTree(t)
	n1 := makeNode("dup", "")
	n2 := makeNode("dup", "")

	if err := tr.Add(n1); err != nil {
		t.Fatalf("unexpected error on first add: %v", err)
	}

	err := tr.Add(n2)
	if err == nil {
		t.Fatal("expected error when adding duplicate ID, got nil")
	}
}

func TestAdd_MissingParent(t *testing.T) {
	tr := freshTree(t)
	child := makeNode("orphan", "nonexistent")

	err := tr.Add(child)
	if err == nil {
		t.Fatal("expected error when adding child with nonexistent parent, got nil")
	}
}

func TestRemove_Leaf(t *testing.T) {
	tr := freshTree(t)
	parent := makeNode("p", "")
	child := makeNode("c", "p")

	if err := tr.Add(parent); err != nil {
		t.Fatalf("unexpected error adding parent: %v", err)
	}
	if err := tr.Add(child); err != nil {
		t.Fatalf("unexpected error adding child: %v", err)
	}

	removed, err := tr.Remove("c")
	if err != nil {
		t.Fatalf("unexpected error removing leaf: %v", err)
	}
	if len(removed) != 1 {
		t.Fatalf("expected 1 removed node, got %d", len(removed))
	}
	if removed[0].ID != "c" {
		t.Fatalf("expected removed node ID 'c', got %q", removed[0].ID)
	}

	if _, ok := tr.Nodes["p"]; !ok {
		t.Fatal("parent should still exist after removing leaf")
	}
	if _, ok := tr.Nodes["c"]; ok {
		t.Fatal("child should be gone after removal")
	}
}

func TestRemove_Recursive(t *testing.T) {
	tr := freshTree(t)
	a := makeNode("A", "")
	b := makeNode("B", "A")
	c := makeNode("C", "B")

	for _, n := range []*Node{a, b, c} {
		if err := tr.Add(n); err != nil {
			t.Fatalf("unexpected error adding node %s: %v", n.ID, err)
		}
	}

	removed, err := tr.Remove("A")
	if err != nil {
		t.Fatalf("unexpected error removing A: %v", err)
	}

	if len(removed) != 3 {
		t.Fatalf("expected 3 removed nodes, got %d", len(removed))
	}

	removedIDs := make(map[string]bool)
	for _, n := range removed {
		removedIDs[n.ID] = true
	}
	for _, id := range []string{"A", "B", "C"} {
		if !removedIDs[id] {
			t.Errorf("expected node %q in removed slice", id)
		}
	}

	if len(tr.Nodes) != 0 {
		t.Fatalf("expected empty tree after recursive remove, got %d nodes", len(tr.Nodes))
	}
}

func TestRemove_NotFound(t *testing.T) {
	tr := freshTree(t)

	_, err := tr.Remove("ghost")
	if err == nil {
		t.Fatal("expected error when removing nonexistent node, got nil")
	}
}

func TestChildren(t *testing.T) {
	tr := freshTree(t)
	parent := makeNode("parent", "")
	c1 := makeNode("c1", "parent")
	c2 := makeNode("c2", "parent")
	c3 := makeNode("c3", "parent")
	unrelated := makeNode("other", "")

	for _, n := range []*Node{parent, c1, c2, c3, unrelated} {
		if err := tr.Add(n); err != nil {
			t.Fatalf("unexpected error adding node %s: %v", n.ID, err)
		}
	}

	children := tr.Children("parent")
	if len(children) != 3 {
		t.Fatalf("expected 3 children, got %d", len(children))
	}

	childIDs := make(map[string]bool)
	for _, c := range children {
		childIDs[c.ID] = true
	}
	for _, id := range []string{"c1", "c2", "c3"} {
		if !childIDs[id] {
			t.Errorf("expected child %q in Children() result", id)
		}
	}
}

func TestRoots(t *testing.T) {
	tr := freshTree(t)
	r1 := makeNode("r1", "")
	r2 := makeNode("r2", "")
	child := makeNode("child", "r1")

	for _, n := range []*Node{r1, r2, child} {
		if err := tr.Add(n); err != nil {
			t.Fatalf("unexpected error adding node %s: %v", n.ID, err)
		}
	}

	roots := tr.Roots()
	if len(roots) != 2 {
		t.Fatalf("expected 2 roots, got %d", len(roots))
	}

	rootIDs := make(map[string]bool)
	for _, r := range roots {
		rootIDs[r.ID] = true
	}
	for _, id := range []string{"r1", "r2"} {
		if !rootIDs[id] {
			t.Errorf("expected root %q in Roots() result", id)
		}
	}
	if rootIDs["child"] {
		t.Error("child node should not appear in Roots()")
	}
}

func TestSaveLoad_RoundTrip(t *testing.T) {
	tr := freshTree(t)
	r := makeNode("root", "")
	c := makeNode("child", "root")

	for _, n := range []*Node{r, c} {
		if err := tr.Add(n); err != nil {
			t.Fatalf("unexpected error adding node %s: %v", n.ID, err)
		}
	}

	if err := tr.Save(); err != nil {
		t.Fatalf("unexpected error saving tree: %v", err)
	}

	loaded, err := LoadTree(tr.path)
	if err != nil {
		t.Fatalf("unexpected error loading tree: %v", err)
	}

	if len(loaded.Nodes) != len(tr.Nodes) {
		t.Fatalf("loaded tree has %d nodes, expected %d", len(loaded.Nodes), len(tr.Nodes))
	}

	for id, original := range tr.Nodes {
		got, ok := loaded.Nodes[id]
		if !ok {
			t.Errorf("node %q missing from loaded tree", id)
			continue
		}
		if got.Name != original.Name {
			t.Errorf("node %q Name = %q, want %q", id, got.Name, original.Name)
		}
		if got.Branch != original.Branch {
			t.Errorf("node %q Branch = %q, want %q", id, got.Branch, original.Branch)
		}
		if got.ParentID != original.ParentID {
			t.Errorf("node %q ParentID = %q, want %q", id, got.ParentID, original.ParentID)
		}
		if got.Status != original.Status {
			t.Errorf("node %q Status = %q, want %q", id, got.Status, original.Status)
		}
	}
}

func TestSaveLoad_RepoDir(t *testing.T) {
	tr := freshTree(t)
	n := makeNode("root", "")
	n.RepoDir = "/some/repo/path"

	if err := tr.Add(n); err != nil {
		t.Fatalf("unexpected error adding node: %v", err)
	}
	if err := tr.Save(); err != nil {
		t.Fatalf("unexpected error saving tree: %v", err)
	}

	loaded, err := LoadTree(tr.path)
	if err != nil {
		t.Fatalf("unexpected error loading tree: %v", err)
	}

	got := loaded.Nodes["root"]
	if got.RepoDir != "/some/repo/path" {
		t.Errorf("RepoDir = %q, want %q", got.RepoDir, "/some/repo/path")
	}
}

func TestSaveLoad_MissingRepoDir(t *testing.T) {
	tr := freshTree(t)
	n := makeNode("old", "")
	// Don't set RepoDir — simulates old state.json

	if err := tr.Add(n); err != nil {
		t.Fatalf("unexpected error adding node: %v", err)
	}
	if err := tr.Save(); err != nil {
		t.Fatalf("unexpected error saving tree: %v", err)
	}

	loaded, err := LoadTree(tr.path)
	if err != nil {
		t.Fatalf("unexpected error loading tree: %v", err)
	}

	got := loaded.Nodes["old"]
	if got.RepoDir != "" {
		t.Errorf("expected empty RepoDir for old node, got %q", got.RepoDir)
	}
}

func TestLoadTree_NonExistent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does_not_exist", "state.json")

	tr, err := LoadTree(path)
	if err != nil {
		t.Fatalf("expected no error for nonexistent file, got: %v", err)
	}

	if tr == nil {
		t.Fatal("expected non-nil tree")
	}
	if len(tr.Nodes) != 0 {
		t.Fatalf("expected empty Nodes map, got %d entries", len(tr.Nodes))
	}
}
