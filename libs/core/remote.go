package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RemoteHost represents a registered remote workspace.
type RemoteHost struct {
	Name     string    `json:"name"`       // e.g. "my-dev-pod"
	SSHHost  string    `json:"ssh_host"`   // e.g. "my-dev-pod.coder"
	WorkDir  string    `json:"work_dir"`   // default working directory on remote
	Template string    `json:"template"`   // coder template used to create
	Created  time.Time `json:"created_at"`
}

// RemoteRegistry manages the set of registered remote hosts.
type RemoteRegistry struct {
	Hosts map[string]*RemoteHost `json:"hosts"`
	path  string
}

// DefaultRemotesPath returns ~/.config/ws/remotes.json.
func DefaultRemotesPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ws", "remotes.json")
}

// LoadRegistry reads the remote registry from disk, or returns an empty registry.
func LoadRegistry(path string) (*RemoteRegistry, error) {
	r := &RemoteRegistry{
		Hosts: make(map[string]*RemoteHost),
		path:  path,
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return r, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read remotes: %w", err)
	}

	if err := json.Unmarshal(data, r); err != nil {
		return nil, fmt.Errorf("parse remotes: %w", err)
	}
	r.path = path
	return r, nil
}

// Save writes the registry to disk.
func (r *RemoteRegistry) Save() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.path, data, 0o644)
}

// Add registers a new remote host.
func (r *RemoteRegistry) Add(host *RemoteHost) error {
	if _, exists := r.Hosts[host.Name]; exists {
		return fmt.Errorf("remote %q already exists", host.Name)
	}
	r.Hosts[host.Name] = host
	return nil
}

// Get returns a remote host by name.
func (r *RemoteRegistry) Get(name string) (*RemoteHost, bool) {
	host, ok := r.Hosts[name]
	return host, ok
}

// Remove deletes a remote host from the registry.
func (r *RemoteRegistry) Remove(name string) error {
	if _, exists := r.Hosts[name]; !exists {
		return fmt.Errorf("remote %q not found", name)
	}
	delete(r.Hosts, name)
	return nil
}
