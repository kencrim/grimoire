package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Tree is the DAG of workstream nodes, persisted to disk.
type Tree struct {
	Nodes map[string]*Node `json:"nodes"`
	path  string
}

// DefaultStatePath returns ~/.config/ws/state.json.
func DefaultStatePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ws", "state.json")
}

// LoadTree reads the tree from disk, or returns an empty tree.
func LoadTree(path string) (*Tree, error) {
	t := &Tree{
		Nodes: make(map[string]*Node),
		path:  path,
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return t, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}

	if err := json.Unmarshal(data, t); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	t.path = path
	return t, nil
}

// Save writes the tree to disk.
func (t *Tree) Save() error {
	if err := os.MkdirAll(filepath.Dir(t.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(t.path, data, 0o644)
}

// Add inserts a node into the tree.
func (t *Tree) Add(n *Node) error {
	if _, exists := t.Nodes[n.ID]; exists {
		return fmt.Errorf("node %q already exists", n.ID)
	}
	if n.ParentID != "" {
		if _, exists := t.Nodes[n.ParentID]; !exists {
			return fmt.Errorf("parent %q not found", n.ParentID)
		}
	}
	t.Nodes[n.ID] = n
	return nil
}

// Remove deletes a node and all its descendants.
func (t *Tree) Remove(id string) ([]*Node, error) {
	if _, exists := t.Nodes[id]; !exists {
		return nil, fmt.Errorf("node %q not found", id)
	}

	var removed []*Node
	var removeRecursive func(string)
	removeRecursive = func(nodeID string) {
		// Find children first
		for _, n := range t.Nodes {
			if n.ParentID == nodeID {
				removeRecursive(n.ID)
			}
		}
		removed = append(removed, t.Nodes[nodeID])
		delete(t.Nodes, nodeID)
	}

	removeRecursive(id)
	return removed, nil
}

// Children returns the direct children of a node.
func (t *Tree) Children(id string) []*Node {
	var children []*Node
	for _, n := range t.Nodes {
		if n.ParentID == id {
			children = append(children, n)
		}
	}
	return children
}

// Roots returns nodes with no parent.
func (t *Tree) Roots() []*Node {
	var roots []*Node
	for _, n := range t.Nodes {
		if n.ParentID == "" {
			roots = append(roots, n)
		}
	}
	return roots
}
