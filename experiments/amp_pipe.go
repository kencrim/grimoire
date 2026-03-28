//go:build ignore

package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func main() {
	fmt.Println("=== Amp Delayed Injection Test ===")

	cmd := exec.Command("amp",
		"--dangerously-allow-all",
		"--stream-json",
		"--stream-json-input",
		"--execute",
	)
	cmd.Dir = os.Getenv("HOME") + "/Documents/GitHub/scriven"

	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		fmt.Printf("FAIL: %v\n", err)
		os.Exit(1)
	}

	// Read stdout in background
	go func() {
		scanner := bufio.NewScanner(stdout)
		buf := make([]byte, 1024*1024)
		scanner.Buffer(buf, len(buf))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(line, "TURN_ONE") || strings.Contains(line, "TURN_TWO") || strings.Contains(line, "\"result\"") {
				fmt.Printf("  [amp] %s\n", line[:min(len(line), 200)])
			}
		}
	}()

	// Send first message
	fmt.Println("\n[test] Sending TURN_ONE...")
	msg1 := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Say exactly: TURN_ONE"}]}}` + "\n"
	stdin.Write([]byte(msg1))

	// Wait for response
	time.Sleep(10 * time.Second)

	// Send second message (the delayed injection)
	fmt.Println("\n[test] Sending TURN_TWO (delayed injection)...")
	msg2 := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"Say exactly: TURN_TWO"}]}}` + "\n"
	stdin.Write([]byte(msg2))

	// Wait for second response
	time.Sleep(10 * time.Second)

	fmt.Println("\n=== Test complete ===")
	stdin.Close()
	cmd.Wait()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
