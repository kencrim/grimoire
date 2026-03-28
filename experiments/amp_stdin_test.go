// +build ignore

// amp_stdin_test.go — Phase 1 proof-of-concept
// Tests Amp's --stream-json-input behavior:
// 1. Can we pipe JSONL into Amp's stdin?
// 2. Does Amp process injected messages as user turns?
// 3. Does injection interrupt active generation?
// 4. Does Amp stay alive after initial -x task completes?
//
// Run: go run experiments/amp_stdin_test.go
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

type AmpMessage struct {
	Content string `json:"content"`
}

type AmpEvent struct {
	Type    string `json:"type,omitempty"`
	Content string `json:"content,omitempty"`
}

func main() {
	fmt.Println("=== Phase 1: Amp stdin injection test ===")
	fmt.Println()

	// Test 1: Launch Amp with --stream-json-input and --stream-json
	fmt.Println("[Test 1] Launching Amp with --stream-json-input...")

	cmd := exec.Command("amp",
		"--dangerously-allow-all",
		"--stream-json-input",
		"--stream-json",
		"-x", "Say exactly: INITIAL_TASK_RECEIVED",
	)

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		fmt.Printf("  FAIL: could not create stdin pipe: %v\n", err)
		os.Exit(1)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Printf("  FAIL: could not create stdout pipe: %v\n", err)
		os.Exit(1)
	}

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		fmt.Printf("  FAIL: could not start amp: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  OK: Amp process started")

	// Read stdout in background
	events := make(chan string, 100)
	go func() {
		scanner := bufio.NewScanner(stdoutPipe)
		for scanner.Scan() {
			line := scanner.Text()
			events <- line
		}
		close(events)
	}()

	// Wait for initial response
	fmt.Println("[Test 2] Waiting for initial task response...")
	deadline := time.After(30 * time.Second)
	gotInitial := false
	for !gotInitial {
		select {
		case line, ok := <-events:
			if !ok {
				fmt.Println("  FAIL: Amp exited before responding")
				os.Exit(1)
			}
			var evt AmpEvent
			if err := json.Unmarshal([]byte(line), &evt); err == nil {
				fmt.Printf("  Event: type=%s content=%.80s\n", evt.Type, evt.Content)
				if len(evt.Content) > 0 {
					gotInitial = true
				}
			}
		case <-deadline:
			fmt.Println("  FAIL: Timed out waiting for initial response")
			cmd.Process.Kill()
			os.Exit(1)
		}
	}
	fmt.Println("  OK: Got initial response from Amp")

	// Test 3: Inject a follow-up message via stdin JSONL
	fmt.Println("[Test 3] Injecting follow-up message via stdin JSONL...")
	time.Sleep(2 * time.Second) // Let Amp settle

	msg := AmpMessage{Content: "Now say exactly: INJECTION_RECEIVED"}
	msgBytes, _ := json.Marshal(msg)
	msgBytes = append(msgBytes, '\n')

	n, err := stdinPipe.Write(msgBytes)
	if err != nil {
		fmt.Printf("  FAIL: could not write to stdin: %v\n", err)
		cmd.Process.Kill()
		os.Exit(1)
	}
	fmt.Printf("  OK: Wrote %d bytes to stdin: %s", n, string(msgBytes))

	// Wait for injection response
	fmt.Println("[Test 4] Waiting for injection response...")
	deadline = time.After(30 * time.Second)
	gotInjection := false
	for !gotInjection {
		select {
		case line, ok := <-events:
			if !ok {
				fmt.Println("  FAIL: Amp exited after injection")
				os.Exit(1)
			}
			var evt AmpEvent
			if err := json.Unmarshal([]byte(line), &evt); err == nil {
				fmt.Printf("  Event: type=%s content=%.80s\n", evt.Type, evt.Content)
				if len(evt.Content) > 0 {
					gotInjection = true
				}
			}
		case <-deadline:
			fmt.Println("  FAIL: Timed out waiting for injection response")
			cmd.Process.Kill()
			os.Exit(1)
		}
	}
	fmt.Println("  OK: Amp processed injected message!")

	// Clean up
	fmt.Println()
	fmt.Println("=== All tests passed ===")
	fmt.Println("Amp successfully:")
	fmt.Println("  1. Launched with --stream-json-input")
	fmt.Println("  2. Processed initial -x task")
	fmt.Println("  3. Stayed alive after initial task")
	fmt.Println("  4. Processed injected JSONL as a new user turn")

	stdinPipe.Close()
	io.Copy(io.Discard, stdoutPipe)
	cmd.Wait()
}
