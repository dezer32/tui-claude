package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vladislav-k/tui-claude/internal/app"
	"github.com/vladislav-k/tui-claude/internal/config"
	"github.com/vladislav-k/tui-claude/internal/ptymanager"
)

func main() {
	claudeDir := flag.String("dir", "", "Claude projects directory (default: ~/.claude/projects)")
	allMode := flag.Bool("all", false, "Show all sessions across all projects")
	flag.Parse()

	cfg := config.DefaultConfig()
	if *claudeDir != "" {
		cfg.ClaudeDir = *claudeDir
	}
	if wd, err := os.Getwd(); err == nil {
		if resolved, err := filepath.EvalSymlinks(wd); err == nil {
			cfg.WorkDir = filepath.Clean(resolved)
		} else {
			cfg.WorkDir = filepath.Clean(wd)
		}
	}
	cfg.AllMode = *allMode

	mgr := ptymanager.NewManager(cfg.PTYBufferSize)
	defer mgr.StopAll()

	m := app.NewModel(cfg, mgr)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
