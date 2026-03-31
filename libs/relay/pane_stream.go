package relay

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"nhooyr.io/websocket"
)

// PaneStreamer handles bidirectional terminal I/O for a tmux pane.
type PaneStreamer struct {
	target   string // tmux pane ID (e.g. %5) or session name
	pipePath string // named pipe for pipe-pane output
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
	// Create a unique named pipe for this pane stream
	safeTarget := strings.ReplaceAll(strings.ReplaceAll(target, "%", "p"), "/", "_")
	pipePath := fmt.Sprintf("%s/ws-pane-%s-%d", os.TempDir(), safeTarget, time.Now().UnixNano())

	return &PaneStreamer{
		target:   target,
		pipePath: pipePath,
	}
}

// StreamTo streams pane output over the WebSocket using tmux pipe-pane.
// Falls back to capture-pane polling if pipe-pane fails.
func (ps *PaneStreamer) StreamTo(ctx context.Context, conn *websocket.Conn) {
	// Try pipe-pane first for real-time streaming
	if err := ps.streamViaPipe(ctx, conn); err != nil {
		log.Printf("[pane-stream] pipe-pane failed for %s, falling back to capture-pane: %v", ps.target, err)
		ps.streamViaCapture(ctx, conn)
	}
}

// streamViaPipe uses tmux pipe-pane to get a real-time byte stream.
func (ps *PaneStreamer) streamViaPipe(ctx context.Context, conn *websocket.Conn) error {
	// Create named pipe (FIFO)
	if err := exec.Command("mkfifo", ps.pipePath).Run(); err != nil {
		return fmt.Errorf("mkfifo: %w", err)
	}
	defer os.Remove(ps.pipePath)

	// Tell tmux to pipe output to our FIFO
	pipeCmd := fmt.Sprintf("cat > %s", ps.pipePath)
	if err := exec.Command("tmux", "pipe-pane", "-o", "-t", ps.target, pipeCmd).Run(); err != nil {
		return fmt.Errorf("pipe-pane: %w", err)
	}

	// Detach pipe-pane when done
	defer exec.Command("tmux", "pipe-pane", "-t", ps.target).Run()

	// Open the FIFO for reading (this blocks until the writer connects)
	// Run in a goroutine so we can cancel via context
	type openResult struct {
		file *os.File
		err  error
	}
	openCh := make(chan openResult, 1)
	go func() {
		f, err := os.Open(ps.pipePath)
		openCh <- openResult{f, err}
	}()

	var file *os.File
	select {
	case <-ctx.Done():
		return ctx.Err()
	case result := <-openCh:
		if result.err != nil {
			return fmt.Errorf("open fifo: %w", result.err)
		}
		file = result.file
	case <-time.After(3 * time.Second):
		// FIFO open timed out — pipe-pane might not have started writing yet.
		// Send one capture-pane frame to prime the connection, then try again.
		return fmt.Errorf("fifo open timeout")
	}
	defer file.Close()

	// Send an initial capture-pane snapshot so the phone sees content immediately
	ps.sendCaptureFrame(ctx, conn)

	// Stream pipe output in chunks
	reader := bufio.NewReaderSize(file, 4096)
	buf := make([]byte, 4096)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		n, err := reader.Read(buf)
		if n > 0 {
			frame := PaneFrame{
				Type:    "frame",
				Content: string(buf[:n]),
			}
			data, _ := json.Marshal(frame)
			if writeErr := conn.Write(ctx, websocket.MessageText, data); writeErr != nil {
				return nil
			}
		}
		if err != nil {
			return fmt.Errorf("read pipe: %w", err)
		}
	}
}

// streamViaCapture falls back to polling capture-pane at ~15fps.
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

// sendCaptureFrame sends a single capture-pane snapshot.
func (ps *PaneStreamer) sendCaptureFrame(ctx context.Context, conn *websocket.Conn) {
	content, cols, rows := ps.capturePaneContent()
	if content == "" {
		return
	}
	frame := PaneFrame{
		Type:    "frame",
		Content: content,
		Cols:    cols,
		Rows:    rows,
	}
	data, _ := json.Marshal(frame)
	conn.Write(ctx, websocket.MessageText, data)
}

// capturePaneContent snapshots the pane using tmux capture-pane.
func (ps *PaneStreamer) capturePaneContent() (content string, cols, rows int) {
	// Capture without -e (no escape sequences) for clean text output.
	// tmux-specific ANSI codes don't translate well to xterm.js.
	cmd := exec.Command("tmux", "capture-pane", "-t", ps.target, "-p")
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
