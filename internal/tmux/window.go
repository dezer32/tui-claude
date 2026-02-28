package tmux

import (
	"fmt"
)

// WindowName returns the tmux window name for a session.
func WindowName(shortID string) string {
	return windowPrefix + shortID
}

// CreateWindow creates a new tmux window running claude --resume.
func CreateWindow(sessionID, shortID, projectPath string) error {
	name := WindowName(shortID)

	// Check if window already exists
	if WindowExists(name) {
		return SelectWindow(name)
	}

	return runSilent(
		"new-window",
		"-n", name,
		"-c", projectPath,
		fmt.Sprintf("claude --resume %s", sessionID),
	)
}

// CreateNewSession creates a new tmux window running a fresh claude session.
func CreateNewSession(projectPath string) error {
	return runSilent(
		"new-window",
		"-n", windowPrefix+"new",
		"-c", projectPath,
		"claude",
	)
}

// SelectWindow switches to an existing tmux window.
func SelectWindow(name string) error {
	return runSilent("select-window", "-t", name)
}

// WindowExists checks if a tmux window with the given name exists.
func WindowExists(name string) bool {
	out, err := run("list-windows", "-F", "#{window_name}")
	if err != nil {
		return false
	}
	for _, line := range splitLines(out) {
		if line == name {
			return true
		}
	}
	return false
}

// CapturPane captures the current content of a tmux pane.
func CapturPane(windowName string) (string, error) {
	return run("capture-pane", "-t", windowName, "-p")
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
