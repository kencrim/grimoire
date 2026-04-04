package relay

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"

	"tailscale.com/ipn/ipnstate"
	"tailscale.com/tsnet"
)

// TailscaleNode wraps a tsnet.Server to give the daemon its own identity
// on the user's Tailnet (e.g., hex.tailnet-name.ts.net).
type TailscaleNode struct {
	server    *tsnet.Server
	port      int
	fqdn      string
	authWrite io.Writer // where to print auth URLs (typically real stderr)
}

// NewTailscaleNode creates an embedded Tailscale node with the given hostname.
// The node's state is persisted to ~/.config/ws/tsnet/ so auth survives restarts.
// authOutput is where auth URLs and interactive prompts are written — pass the
// real os.Stderr (before any redirect) so the user sees them in the terminal.
func NewTailscaleNode(hostname string, port int, authOutput io.Writer) *TailscaleNode {
	home, _ := os.UserHomeDir()
	stateDir := filepath.Join(home, ".config", "ws", "tsnet")
	os.MkdirAll(stateDir, 0o755)

	node := &TailscaleNode{
		port:      port,
		authWrite: authOutput,
	}

	srv := &tsnet.Server{
		Hostname: hostname,
		Dir:      stateDir,
		Logf: func(format string, args ...any) {
			msg := fmt.Sprintf(format, args...)
			log.Printf("[tsnet] %s", msg)
			// Also print auth-related messages to the terminal
			if node.authWrite != nil && (strings.Contains(msg, "https://") || strings.Contains(msg, "login")) {
				fmt.Fprintf(node.authWrite, "[tsnet] %s\n", msg)
			}
		},
	}
	node.server = srv

	// TS_AUTHKEY is read automatically by tsnet, but we log whether it's set
	if os.Getenv("TS_AUTHKEY") != "" {
		log.Printf("[tsnet] using TS_AUTHKEY for authentication")
	}

	return node
}

// NeedsAuth returns true if the tsnet state directory is empty (first run).
func (t *TailscaleNode) NeedsAuth() bool {
	entries, err := os.ReadDir(t.server.Dir)
	if err != nil {
		return true
	}
	return len(entries) == 0
}

// Up starts the Tailscale node and waits for it to be connected.
// On first run this triggers an interactive browser auth flow (or uses TS_AUTHKEY).
func (t *TailscaleNode) Up(ctx context.Context) (*ipnstate.Status, error) {
	status, err := t.server.Up(ctx)
	if err != nil {
		return nil, fmt.Errorf("tsnet up: %w", err)
	}

	// Cache the FQDN
	domains := t.server.CertDomains()
	if len(domains) > 0 {
		t.fqdn = domains[0]
	}

	log.Printf("[tsnet] connected as %s (IPs: %v)", t.fqdn, status.TailscaleIPs)
	return status, nil
}

// Listen returns a net.Listener on the tsnet virtual interface.
func (t *TailscaleNode) Listen() (net.Listener, error) {
	addr := fmt.Sprintf(":%d", t.port)
	ln, err := t.server.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("tsnet listen %s: %w", addr, err)
	}
	log.Printf("[tsnet] listening on %s:%d", t.fqdn, t.port)
	return ln, nil
}

// FQDN returns the Tailscale DNS name (e.g., hex.tailnet-name.ts.net).
// Only valid after a successful Up() call.
func (t *TailscaleNode) FQDN() string {
	return t.fqdn
}

// Close shuts down the embedded Tailscale node.
func (t *TailscaleNode) Close() error {
	return t.server.Close()
}
