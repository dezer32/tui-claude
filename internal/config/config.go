package config

import (
	"os"
	"path/filepath"
)

// Config holds application configuration.
type Config struct {
	ClaudeDir      string
	PreviewEnabled bool
	LiveInterval   int   // seconds between live preview refreshes
	MaxJSONLSize   int64 // max JSONL file size to parse (bytes)
	MaxMessages    int   // max messages to show in preview
	PTYBufferSize  int   // ring buffer size for PTY capture (bytes)
	WorkDir        string // current working directory (empty = all sessions)
	AllMode        bool   // show all sessions across all projects
}

// DefaultConfig returns config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		ClaudeDir:      defaultClaudeDir(),
		PreviewEnabled: true,
		LiveInterval:   2,
		MaxJSONLSize:   10 * 1024 * 1024, // 10MB
		MaxMessages:    50,
		PTYBufferSize:  16384, // 16KB
	}
}

func defaultClaudeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects")
}
