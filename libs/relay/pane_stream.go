package relay

import (
	"context"
	"encoding/json"
	"log"
	"os/exec"
	"strings"
	"time"

	"nhooyr.io/websocket"
)

// PaneStreamer handles bidirectional terminal I/O for a tmux pane.
type PaneStreamer struct {
	target    string   // tmux pane ID (e.g. %5) or session name
	prevLines []string // previous frame's lines for scroll detection
}

// PaneFrame is a captured terminal snapshot sent to the phone.
type PaneFrame struct {
	Type     string `json:"type"`     // frame
	Content  string `json:"content"`  // ANSI-encoded terminal content
	Cols     int    `json:"cols"`
	Rows     int    `json:"rows"`
	Scrolled int    `json:"scrolled"` // -1 = full snapshot, 0 = in-place update, >0 = lines scrolled off top
}

// NewPaneStreamer creates a streamer for a tmux pane.
func NewPaneStreamer(target string) *PaneStreamer {
	return &PaneStreamer{
		target: target,
	}
}

// StreamTo streams pane output over the WebSocket using capture-pane polling.
// Each frame is a full snapshot of the pane content with ANSI escape sequences
// preserved, so the phone gets an accurate viewport into the desktop terminal.
func (ps *PaneStreamer) StreamTo(ctx context.Context, conn *websocket.Conn) {
	ps.streamViaCapture(ctx, conn)
}

// streamViaCapture polls capture-pane at ~15fps.
// It detects when content scrolls (new lines at the bottom) and sends a scroll
// offset so the phone can push old lines into xterm.js scrollback naturally.
func (ps *PaneStreamer) streamViaCapture(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(66 * time.Millisecond)
	defer ticker.Stop()

	var lastContent string

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			content, cols, rows := ps.capturePaneContent()
			if content == "" || content == lastContent {
				continue
			}
			lastContent = content

			lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")

			scrolled := -1 // default: full snapshot (first frame or dimension change)
			if ps.prevLines != nil && len(lines) == len(ps.prevLines) {
				scrolled = ps.detectScroll(ps.prevLines, lines)
			}
			ps.prevLines = lines

			frame := PaneFrame{
				Type:     "frame",
				Content:  content,
				Cols:     cols,
				Rows:     rows,
				Scrolled: scrolled,
			}
			data, _ := json.Marshal(frame)
			if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
				return
			}
		}
	}
}

// detectScroll checks if newLines is a scrolled version of prevLines.
// Returns the number of lines scrolled off the top (0 = in-place change only).
func (ps *PaneStreamer) detectScroll(prevLines, newLines []string) int {
	n := len(newLines)

	// Try offsets 1..maxCheck: if newLines[0:n-offset] matches prevLines[offset:n],
	// then 'offset' lines scrolled off the top.
	maxCheck := 20
	if maxCheck > n/2 {
		maxCheck = n / 2
	}

	for offset := 1; offset <= maxCheck; offset++ {
		match := true
		// Check the overlapping region. Ignore the bottom 3 lines since
		// status bars (Claude Code prompt line, etc.) change independently of scroll.
		checkEnd := n - offset - 3
		if checkEnd < 3 {
			continue
		}
		for i := 0; i < checkEnd; i++ {
			if newLines[i] != prevLines[i+offset] {
				match = false
				break
			}
		}
		if match {
			return offset
		}
	}
	return 0
}

// capturePaneContent snapshots the pane using tmux capture-pane with ANSI codes preserved.
func (ps *PaneStreamer) capturePaneContent() (content string, cols, rows int) {
	// Capture with -e to preserve ANSI escape sequences (colors, formatting).
	// The phone's xterm.js renders these natively.
	cmd := exec.Command("tmux", "capture-pane", "-e", "-t", ps.target, "-p")
	out, err := cmd.Output()
	if err != nil {
		return "", 0, 0
	}

	dimCmd := exec.Command("tmux", "display-message", "-t", ps.target, "-p", "#{pane_width} #{pane_height}")
	dimOut, err := dimCmd.Output()
	if err != nil {
		return string(out), 0, 0
	}

	parts := strings.Fields(strings.TrimSpace(string(dimOut)))
	if len(parts) == 2 {
		parseDim := func(s string) int {
			var n int
			for _, c := range s {
				n = n*10 + int(c-'0')
			}
			return n
		}
		cols = parseDim(parts[0])
		rows = parseDim(parts[1])
	}

	return string(out), cols, rows
}

// HandleInput processes input from the phone and sends it to the tmux pane.
func (ps *PaneStreamer) HandleInput(input PaneInputMsg) error {
	switch input.Type {
	case "input":
		if input.Data == "" {
			return nil
		}
		cmd := exec.Command("tmux", "send-keys", "-l", "-t", ps.target, input.Data)
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("[pane-stream] send-keys -l error: %s", string(out))
			return err
		}

	case "special":
		if input.Data == "" {
			return nil
		}
		cmd := exec.Command("tmux", "send-keys", "-t", ps.target, input.Data)
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("[pane-stream] send-keys special error: %s", string(out))
			return err
		}

	case "resize":
		log.Printf("[pane-stream] phone resize: %dx%d", input.Cols, input.Rows)

	default:
		log.Printf("[pane-stream] unknown input type: %s", input.Type)
	}

	return nil
}
