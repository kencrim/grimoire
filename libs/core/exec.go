package core

import (
	"os/exec"
	"strings"
)

// RunOnHost builds an exec.Cmd that runs locally (host="") or via SSH.
// For remote hosts, the command is wrapped in: ssh -o BatchMode=yes <host> <shell-escaped args>
func RunOnHost(host string, name string, args ...string) *exec.Cmd {
	if host == "" {
		return exec.Command(name, args...)
	}

	// Build the remote command string with proper shell escaping
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, name)
	parts = append(parts, shellEscapeArgs(args)...)

	remoteCmd := strings.Join(parts, " ")

	return exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", host, remoteCmd)
}

// RunOnHostInteractive builds an exec.Cmd for interactive SSH sessions (allocates a TTY).
// Used for commands like tmux attach-session that need a terminal.
func RunOnHostInteractive(host string, name string, args ...string) *exec.Cmd {
	if host == "" {
		return exec.Command(name, args...)
	}

	sshArgs := []string{"-t", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", host, name}
	sshArgs = append(sshArgs, args...)

	return exec.Command("ssh", sshArgs...)
}

// shellEscapeArgs escapes arguments for safe passage through SSH.
// Each argument that contains special characters is wrapped in single quotes.
func shellEscapeArgs(args []string) []string {
	escaped := make([]string, len(args))
	for i, arg := range args {
		escaped[i] = shellEscape(arg)
	}
	return escaped
}

// shellEscape wraps a string in single quotes if it contains shell-special characters.
// Single quotes within the string are handled by ending the quote, inserting an
// escaped single quote, and re-opening the quote: '\''
func shellEscape(s string) string {
	// Safe characters that don't need quoting
	safe := true
	for _, c := range s {
		if !isShellSafe(c) {
			safe = false
			break
		}
	}
	if safe && s != "" {
		return s
	}

	// Wrap in single quotes, escaping any embedded single quotes
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func isShellSafe(c rune) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '_' || c == '.' || c == '/' ||
		c == ':' || c == '@' || c == '%' || c == '+'
}
