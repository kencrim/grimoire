package relay

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
)

// ShadowManager manages shadow tmux sessions for mobile clients.
// Shadow sessions are linked to the original session but have independent dimensions.
type ShadowManager struct {
	mu       sync.Mutex
	sessions map[string]*ShadowSession // keyed by original session name
}

// ShadowSession tracks a shadow tmux session linked to an original.
type ShadowSession struct {
	OriginalSession string
	ShadowName      string
	Clients         int // reference count of connected phone clients
}

// NewShadowManager creates a shadow session manager.
func NewShadowManager() *ShadowManager {
	return &ShadowManager{
		sessions: make(map[string]*ShadowSession),
	}
}

// Acquire creates or reuses a shadow session for the given original session.
// Returns the shadow session name. Call Release when the client disconnects.
func (sm *ShadowManager) Acquire(originalSession string) (string, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if shadow, ok := sm.sessions[originalSession]; ok {
		shadow.Clients++
		log.Printf("[shadow] reusing shadow session %s (clients: %d)", shadow.ShadowName, shadow.Clients)
		return shadow.ShadowName, nil
	}

	shadowName := originalSession + "-phone"

	// Enable aggressive-resize so linked sessions can have independent dimensions
	exec.Command("tmux", "set", "-g", "aggressive-resize", "on").Run()

	// Kill any stale shadow session from a previous run
	exec.Command("tmux", "kill-session", "-t", shadowName).Run()

	// Create linked session — shares the same window group but can have its own size
	cmd := exec.Command("tmux", "new-session", "-d", "-t", originalSession, "-s", shadowName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("create shadow session %s: %s", shadowName, string(out))
	}

	sm.sessions[originalSession] = &ShadowSession{
		OriginalSession: originalSession,
		ShadowName:      shadowName,
		Clients:         1,
	}

	log.Printf("[shadow] created shadow session %s for %s", shadowName, originalSession)
	return shadowName, nil
}

// Release decrements the client count for a shadow session.
// When no clients remain, the shadow session is destroyed.
func (sm *ShadowManager) Release(originalSession string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	shadow, ok := sm.sessions[originalSession]
	if !ok {
		return
	}

	shadow.Clients--
	if shadow.Clients <= 0 {
		// Kill the shadow session
		exec.Command("tmux", "kill-session", "-t", shadow.ShadowName).Run()
		delete(sm.sessions, originalSession)
		log.Printf("[shadow] destroyed shadow session %s", shadow.ShadowName)
	} else {
		log.Printf("[shadow] released shadow session %s (clients: %d)", shadow.ShadowName, shadow.Clients)
	}
}

// Resize resizes a shadow session to the phone's terminal dimensions.
func (sm *ShadowManager) Resize(originalSession string, cols, rows int) error {
	sm.mu.Lock()
	shadow, ok := sm.sessions[originalSession]
	sm.mu.Unlock()

	if !ok {
		return fmt.Errorf("no shadow session for %s", originalSession)
	}

	cmd := exec.Command("tmux", "resize-window", "-t", shadow.ShadowName,
		"-x", fmt.Sprintf("%d", cols), "-y", fmt.Sprintf("%d", rows))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("resize shadow %s: %s", shadow.ShadowName, string(out))
	}

	log.Printf("[shadow] resized %s to %dx%d", shadow.ShadowName, cols, rows)
	return nil
}

// PaneInShadow returns the equivalent pane ID in the shadow session.
// Since shadow sessions share window groups, panes have the same IDs.
// But we need to find the pane by index if the IDs differ.
func (sm *ShadowManager) PaneInShadow(originalSession, originalPaneID string) string {
	sm.mu.Lock()
	shadow, ok := sm.sessions[originalSession]
	sm.mu.Unlock()

	if !ok {
		return originalPaneID
	}

	// List panes in original session to find the index of our pane
	origCmd := exec.Command("tmux", "list-panes", "-t", originalSession, "-F", "#{pane_id}")
	origOut, err := origCmd.Output()
	if err != nil {
		return originalPaneID
	}

	origPanes := strings.Split(strings.TrimSpace(string(origOut)), "\n")
	paneIndex := -1
	for i, p := range origPanes {
		if strings.TrimSpace(p) == originalPaneID {
			paneIndex = i
			break
		}
	}

	if paneIndex < 0 {
		return originalPaneID
	}

	// Get the corresponding pane in the shadow session
	shadowCmd := exec.Command("tmux", "list-panes", "-t", shadow.ShadowName, "-F", "#{pane_id}")
	shadowOut, err := shadowCmd.Output()
	if err != nil {
		return originalPaneID
	}

	shadowPanes := strings.Split(strings.TrimSpace(string(shadowOut)), "\n")
	if paneIndex < len(shadowPanes) {
		return strings.TrimSpace(shadowPanes[paneIndex])
	}

	return originalPaneID
}

// CleanupAll kills all shadow sessions. Called on daemon shutdown.
func (sm *ShadowManager) CleanupAll() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	for _, shadow := range sm.sessions {
		exec.Command("tmux", "kill-session", "-t", shadow.ShadowName).Run()
		log.Printf("[shadow] cleanup: destroyed %s", shadow.ShadowName)
	}
	sm.sessions = make(map[string]*ShadowSession)
}
