package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vladislav-k/tui-claude/internal/app"
	"github.com/vladislav-k/tui-claude/internal/config"
)

func main() {
	claudeDir := flag.String("dir", "", "Claude projects directory (default: ~/.claude/projects)")
	flag.Parse()

	cfg := config.DefaultConfig()
	if *claudeDir != "" {
		cfg.ClaudeDir = *claudeDir
	}

	m := app.NewModel(cfg)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
