package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// applyShader rewrites the Ghostty config to activate the given background shader
// and triggers a config reload. Uses a lock file to prevent concurrent writes.
func applyShader(shaderFile string) error {
	configPath := filepath.Join(os.Getenv("HOME"), ".config", "ghostty", "config")
	lockPath := configPath + ".lock"

	// Acquire lock file
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create lock: %w", err)
	}
	defer lock.Close()
	defer os.Remove(lockPath)

	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read ghostty config: %w", err)
	}

	// Safety check: don't process an empty or near-empty file
	if len(data) < 50 {
		return fmt.Errorf("ghostty config appears empty or corrupt (%d bytes), refusing to write", len(data))
	}

	lines := strings.Split(string(data), "\n")
	var out []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Only touch lines that reference known background shaders
		if shaderName, ok := matchBackgroundShader(trimmed); ok {
			if shaderName == shaderFile {
				out = append(out, "custom-shader = shaders/"+shaderFile)
			} else {
				out = append(out, "# custom-shader = shaders/"+shaderName)
			}
			continue
		}

		// Pass everything else through unchanged
		out = append(out, line)
	}

	result := strings.Join(out, "\n")

	// Safety check: don't write if result is shorter than original by more than 20%
	if len(result) < len(data)*80/100 {
		return fmt.Errorf("shader swap would shrink config from %d to %d bytes, aborting", len(data), len(result))
	}

	if err := os.WriteFile(configPath, []byte(result), 0o644); err != nil {
		return fmt.Errorf("write ghostty config: %w", err)
	}

	// Trigger Ghostty config reload via menu bar click
	exec.Command("osascript", "-e",
		`tell application "System Events" to tell application process "ghostty" to click menu item "Reload Configuration" of menu "Ghostty" of menu bar 1`).Run()

	return nil
}

// Known background shaders that we rotate between workstreams.
var knownBackgroundShaders = map[string]bool{
	"animated-gradient-shader.glsl": true,
	"starfield.glsl":                true,
	"inside-the-matrix.glsl":        true,
	"sparks-from-fire.glsl":         true,
	"just-snow.glsl":                true,
	"gears-and-belts.glsl":          true,
	"cubes.glsl":                    true,
}

// matchBackgroundShader checks if a config line references a known background shader.
// Returns the shader filename and true if matched, empty and false otherwise.
func matchBackgroundShader(line string) (string, bool) {
	// Try active form: "custom-shader = shaders/starfield.glsl"
	if strings.HasPrefix(line, "custom-shader = shaders/") {
		name := strings.TrimPrefix(line, "custom-shader = shaders/")
		if knownBackgroundShaders[name] {
			return name, true
		}
		return "", false
	}

	// Try commented form: "# custom-shader = shaders/starfield.glsl"
	if strings.HasPrefix(line, "# custom-shader = shaders/") {
		name := strings.TrimPrefix(line, "# custom-shader = shaders/")
		if knownBackgroundShaders[name] {
			return name, true
		}
		return "", false
	}

	return "", false
}
