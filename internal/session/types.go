package session

import "time"

// Session represents a single Claude Code session.
type Session struct {
	SessionID   string    `json:"sessionId"`
	FullPath    string    `json:"fullPath"`
	FileMtime   int64     `json:"fileMtime"`
	FirstPrompt string    `json:"firstPrompt"`
	Summary     string    `json:"summary"`
	MsgCount    int       `json:"messageCount"`
	Created     time.Time `json:"created"`
	Modified    time.Time `json:"modified"`
	GitBranch   string    `json:"gitBranch"`
	ProjectPath string    `json:"projectPath"`
	IsSidechain bool      `json:"isSidechain"`

	// Runtime fields (not from JSON)
	ProjectName string `json:"-"`
	IsRunning   bool   `json:"-"`
	IsArchived  bool   `json:"-"`
	Selected    bool   `json:"-"`
}

// DisplayTitle returns summary if available, otherwise truncated firstPrompt.
func (s Session) DisplayTitle() string {
	if s.Summary != "" {
		return s.Summary
	}
	if len(s.FirstPrompt) > 60 {
		return s.FirstPrompt[:57] + "..."
	}
	return s.FirstPrompt
}

// ShortID returns first 8 chars of sessionId.
func (s Session) ShortID() string {
	if len(s.SessionID) >= 8 {
		return s.SessionID[:8]
	}
	return s.SessionID
}

// Age returns human-readable time since last modification.
func (s Session) Age() string {
	d := time.Since(s.Modified)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return formatDuration(int(d.Minutes()), "m")
	case d < 24*time.Hour:
		return formatDuration(int(d.Hours()), "h")
	default:
		return formatDuration(int(d.Hours()/24), "d")
	}
}

func formatDuration(val int, suffix string) string {
	if val == 0 {
		return "now"
	}
	return itoa(val) + suffix
}

func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return itoa(i/10) + string(rune('0'+i%10))
}

// Project groups sessions by project path.
type Project struct {
	Name     string
	Path     string
	Sessions []Session
}

// SessionIndex represents the sessions-index.json file format.
type SessionIndex struct {
	Version int       `json:"version"`
	Entries []Session `json:"entries"`
}

// SortField defines sort options.
type SortField int

const (
	SortByDate SortField = iota
	SortByProject
	SortByMessages
)

func (s SortField) String() string {
	switch s {
	case SortByDate:
		return "date"
	case SortByProject:
		return "project"
	case SortByMessages:
		return "messages"
	default:
		return "date"
	}
}

func (s SortField) Next() SortField {
	return (s + 1) % 3
}
