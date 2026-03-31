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
	target string // tmux pane ID (e.g. %5) or session name
}

// PaneFrame is a captured terminal snapshot sent to the phone.
type PaneFrame struct {
	Type    string `json:"type"`    // frame
	Content string `json:"content"` // ANSI-encoded terminal content
	Cols    int    `json:"cols"`
	Rows    int    `json:"rows"`
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

			frame := PaneFrame{
				Type:    "frame",
				Content: content,
				Cols:    cols,
				Rows:    rows,
			}
			data, _ := json.Marshal(frame)
			if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
				return
			}
		}
	}
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
