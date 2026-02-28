package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Delete removes a session's JSONL file and updates the index.
func Delete(s Session) error {
	// Remove JSONL file
	if err := os.Remove(s.FullPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove session file: %w", err)
	}

	// Update index
	return removeFromIndex(s)
}

// Rename updates the summary in the sessions-index.json.
func Rename(s Session, newSummary string) error {
	indexPath := indexPathForSession(s)

	data, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}

	var index SessionIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return err
	}

	for i := range index.Entries {
		if index.Entries[i].SessionID == s.SessionID {
			index.Entries[i].Summary = newSummary
			break
		}
	}

	return writeIndex(indexPath, index)
}

// Export writes session messages to a markdown file.
func Export(s Session, outputDir string, maxSize int64, maxMessages int) (string, error) {
	messages, err := ParseJSONL(s.FullPath, maxSize, maxMessages)
	if err != nil {
		return "", err
	}

	filename := fmt.Sprintf("session-%s.md", s.ShortID())
	outputPath := filepath.Join(outputDir, filename)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Session: %s\n\n", s.DisplayTitle()))
	sb.WriteString(fmt.Sprintf("- **Project**: %s\n", s.ProjectPath))
	sb.WriteString(fmt.Sprintf("- **Branch**: %s\n", s.GitBranch))
	sb.WriteString(fmt.Sprintf("- **Created**: %s\n", s.Created.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("- **Modified**: %s\n", s.Modified.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("- **Messages**: %d\n\n", s.MsgCount))
	sb.WriteString("---\n\n")

	for _, msg := range messages {
		switch msg.Type {
		case "user":
			sb.WriteString("## User\n\n")
			sb.WriteString(msg.Content + "\n\n")
		case "assistant":
			sb.WriteString("## Assistant\n\n")
			sb.WriteString(msg.Content + "\n\n")
		case "summary":
			sb.WriteString("## Summary\n\n")
			sb.WriteString("_" + msg.Content + "_\n\n")
		}
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", err
	}

	return outputPath, os.WriteFile(outputPath, []byte(sb.String()), 0o644)
}

// Archive moves session file to an archive directory.
func Archive(s Session) error {
	archiveDir := filepath.Join(filepath.Dir(s.FullPath), "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return err
	}

	dest := filepath.Join(archiveDir, filepath.Base(s.FullPath))
	if err := os.Rename(s.FullPath, dest); err != nil {
		return err
	}

	return removeFromIndex(s)
}

func removeFromIndex(s Session) error {
	indexPath := indexPathForSession(s)

	data, err := os.ReadFile(indexPath)
	if err != nil {
		return err
	}

	var index SessionIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return err
	}

	var remaining []Session
	for _, e := range index.Entries {
		if e.SessionID != s.SessionID {
			remaining = append(remaining, e)
		}
	}
	index.Entries = remaining

	return writeIndex(indexPath, index)
}

func writeIndex(path string, index SessionIndex) error {
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func indexPathForSession(s Session) string {
	return filepath.Join(filepath.Dir(s.FullPath), "sessions-index.json")
}
