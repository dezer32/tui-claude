package tmux

import (
	"os/exec"
	"strings"
)

const windowPrefix = "tui-claude:"

// IsAvailable checks if tmux binary exists.
func IsAvailable() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// IsInsideSession checks if we're running inside a tmux session.
func IsInsideSession() bool {
	out, err := run("display-message", "-p", "#{session_name}")
	return err == nil && out != ""
}

// CurrentSession returns the name of the current tmux session.
func CurrentSession() string {
	out, _ := run("display-message", "-p", "#{session_name}")
	return out
}

// run executes a tmux command and returns trimmed output.
func run(args ...string) (string, error) {
	cmd := exec.Command("tmux", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// runSilent executes a tmux command ignoring output.
func runSilent(args ...string) error {
	cmd := exec.Command("tmux", args...)
	return cmd.Run()
}
