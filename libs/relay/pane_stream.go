package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"nhooyr.io/websocket"
)

// PaneStreamer handles bidirectional terminal I/O for a tmux pane.
type PaneStreamer struct {
	target        string        // tmux pane ID (e.g. %5) or session name
	host          string        // SSH host for remote agents (empty = local)
	interval      time.Duration // polling interval (lower for remote to reduce SSH overhead)
	prevStripped  []string      // previous frame's lines with ANSI stripped (for scroll comparison)
}

// PaneFrame is a captured terminal snapshot sent to the phone.
type PaneFrame struct {
	Type     string `json:"type"`     // frame
	Content  string `json:"content"`  // ANSI-encoded terminal content
	Cols     int    `json:"cols"`
	Rows     int    `json:"rows"`
	Scrolled int    `json:"scrolled"` // -1 = full snapshot, 0 = in-place update, >0 = lines scrolled off top
}

// waitForPrompt waits for the target pane's cursor to settle on a prompt-like line.
// This prevents rapid-fire input_submit calls from piling keys into tmux faster
// than the application (e.g. Claude Code) can consume them.
func (ps *PaneStreamer) waitForPrompt() {
	// Poll the pane's cursor line until it looks like a prompt or we timeout.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		cmd := runOnHost(ps.host, "tmux", "capture-pane", "-t", ps.target, "-p", "-T")
		out, err := cmd.CombinedOutput()
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		// Check all lines for common prompt indicators.
		// Claude Code's prompt line is "❯" followed by spaces and the Rustmurmur owl,
		// so we check if any line starts with a prompt character.
		lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
		for _, line := range lines {
			stripped := strings.TrimSpace(stripAnsi(line))
			if stripped == "" {
				continue
			}
			if strings.HasPrefix(stripped, "❯") ||
				strings.HasPrefix(stripped, "$ ") || stripped == "$" ||
				strings.HasPrefix(stripped, "> ") || stripped == ">" ||
				strings.HasPrefix(stripped, "% ") || stripped == "%" {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	// Timeout — proceed anyway to avoid hanging forever
	log.Printf("[pane-stream] waitForPrompt: timed out after 5s for target %s", ps.target)
}

// NewPaneStreamer creates a streamer for a tmux pane.
// For remote panes, the polling rate is reduced to ~3fps to account for SSH overhead.
func NewPaneStreamer(target string, host string) *PaneStreamer {
	interval := 66 * time.Millisecond // ~15fps for local
	if host != "" {
		interval = 333 * time.Millisecond // ~3fps for remote
	}
	return &PaneStreamer{
		target:   target,
		host:     host,
		interval: interval,
	}
}

// StreamTo streams pane output over the WebSocket using capture-pane polling.
// First sends the scrollback history so the phone can scroll up through past output,
// then polls the visible pane at ~15fps for live updates.
func (ps *PaneStreamer) StreamTo(ctx context.Context, conn *websocket.Conn) {
	// Send scrollback history first — this fills xterm.js scrollback buffer
	// so the user can scroll up through past output.
	ps.sendHistory(ctx, conn)
	// Then start live polling of the visible pane
	ps.streamViaCapture(ctx, conn)
}

// sendHistory captures the tmux scrollback buffer and sends it as a single frame.
// Sent without cols/rows so the phone writes it incrementally (no screen clear),
// allowing the content to naturally flow into xterm.js scrollback.
func (ps *PaneStreamer) sendHistory(ctx context.Context, conn *websocket.Conn) {
	cmd := runOnHost(ps.host, "tmux", "capture-pane", "-e", "-t", ps.target, "-p", "-S", "-1000")
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return
	}

	// Send without cols/rows — the phone's JS treats frames without dimensions
	// as incremental writes (no clear), so the content flows into scrollback.
	frame := PaneFrame{
		Type:     "frame",
		Content:  string(out),
		Scrolled: -1,
	}
	data, _ := json.Marshal(frame)
	conn.Write(ctx, websocket.MessageText, data)
}

// streamViaCapture polls capture-pane at ~15fps.
// It detects when content scrolls (new lines at the bottom) and sends a scroll
// offset so the phone can push old lines into xterm.js scrollback naturally.
func (ps *PaneStreamer) streamViaCapture(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(ps.interval)
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
			stripped := make([]string, len(lines))
			for i, l := range lines {
				stripped[i] = stripAnsi(l)
			}

			scrolled := -1 // default: full snapshot (first frame or dimension change)
			if ps.prevStripped != nil && len(stripped) == len(ps.prevStripped) {
				scrolled = detectScroll(ps.prevStripped, stripped)
			}
			ps.prevStripped = stripped

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
// Both slices should have ANSI codes already stripped for reliable comparison.
// Returns the number of lines scrolled off the top (0 = in-place change only).
func detectScroll(prevLines, newLines []string) int {
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

// stripAnsi removes ANSI escape sequences from a string for comparison purposes.
func stripAnsi(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '\033' {
			// Skip CSI sequences: ESC [ ... final_byte
			if i+1 < len(s) && s[i+1] == '[' {
				j := i + 2
				for j < len(s) && s[j] >= 0x30 && s[j] <= 0x3F {
					j++ // parameter bytes
				}
				for j < len(s) && s[j] >= 0x20 && s[j] <= 0x2F {
					j++ // intermediate bytes
				}
				if j < len(s) {
					j++ // final byte
				}
				i = j
				continue
			}
			// Skip other ESC sequences (ESC + one byte)
			i += 2
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// capturePaneContent snapshots the pane using tmux capture-pane with ANSI codes preserved.
func (ps *PaneStreamer) capturePaneContent() (content string, cols, rows int) {
	// Capture with -e to preserve ANSI escape sequences (colors, formatting).
	// The phone's xterm.js renders these natively.
	cmd := runOnHost(ps.host, "tmux", "capture-pane", "-e", "-t", ps.target, "-p")
	out, err := cmd.Output()
	if err != nil {
		return "", 0, 0
	}

	dimCmd := runOnHost(ps.host, "tmux", "display-message", "-t", ps.target, "-p", "#{pane_width} #{pane_height}")
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
		cmd := runOnHost(ps.host, "tmux", "send-keys", "-l", "-t", ps.target, input.Data)
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("[pane-stream] send-keys -l error: %s", string(out))
			return err
		}

	case "input_submit":
		// Send text + Enter, then wait for the application to show a prompt again
		// before returning. This prevents the next queued input_submit from typing
		// into a pane that hasn't finished processing the previous Enter.
		if input.Data == "" {
			return nil
		}
		log.Printf("[pane-stream] input_submit: target=%s len=%d data=%q", ps.target, len(input.Data), input.Data)

		cmd := runOnHost(ps.host, "tmux", "send-keys", "-l", "-t", ps.target, input.Data)
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("[pane-stream] send-keys -l error: %s", string(out))
			return err
		}
		enterCmd := runOnHost(ps.host, "tmux", "send-keys", "-t", ps.target, "Enter")
		if out, err := enterCmd.CombinedOutput(); err != nil {
			log.Printf("[pane-stream] send-keys Enter error: %s", string(out))
			return err
		}
		log.Printf("[pane-stream] input_submit: sent, waiting for prompt")

		// First wait briefly for the pane to start processing (prompt disappears)
		time.Sleep(200 * time.Millisecond)
		// Then wait for prompt to reappear
		ps.waitForPrompt()
		log.Printf("[pane-stream] input_submit: prompt detected, ready for next")

	case "special":
		if input.Data == "" {
			return nil
		}
		cmd := runOnHost(ps.host, "tmux", "send-keys", "-t", ps.target, input.Data)
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("[pane-stream] send-keys special error: %s", string(out))
			return err
		}

	case "resize":
		log.Printf("[pane-stream] phone resize: %dx%d", input.Cols, input.Rows)
		if input.Cols > 0 && input.Rows > 0 {
			cmd := runOnHost(ps.host, "tmux", "resize-pane", "-t", ps.target,
				"-x", fmt.Sprintf("%d", input.Cols), "-y", fmt.Sprintf("%d", input.Rows))
			if out, err := cmd.CombinedOutput(); err != nil {
				log.Printf("[pane-stream] resize-pane error: %s", string(out))
				return err
			}
		}

	default:
		log.Printf("[pane-stream] unknown input type: %s", input.Type)
	}

	return nil
}
