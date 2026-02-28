package tmux

import (
	"strings"
)

// DetectRunning returns a map of session IDs that have running tmux windows.
func DetectRunning() map[string]bool {
	running := make(map[string]bool)

	out, err := run("list-windows", "-F", "#{window_name}")
	if err != nil {
		return running
	}

	for _, name := range splitLines(out) {
		if strings.HasPrefix(name, windowPrefix) {
			shortID := strings.TrimPrefix(name, windowPrefix)
			if shortID != "" && shortID != "new" {
				running[shortID] = true
			}
		}
	}

	return running
}

// MatchRunning matches short IDs from DetectRunning against full session IDs.
func MatchRunning(shortIDs map[string]bool, sessionIDs []string) map[string]bool {
	fullMatch := make(map[string]bool)
	for _, id := range sessionIDs {
		if len(id) >= 8 && shortIDs[id[:8]] {
			fullMatch[id] = true
		}
	}
	return fullMatch
}
