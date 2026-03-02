package app

import (
	"github.com/vladislav-k/tui-claude/internal/session"
)

// SessionsLoadedMsg is sent when sessions are loaded from disk.
type SessionsLoadedMsg struct {
	Sessions []session.Session
	Projects []session.Project
	Err      error
}

// SessionSelectedMsg is sent when user selects a session from the list.
type SessionSelectedMsg struct {
	Session session.Session
}

// SessionResumedMsg is sent when a session is resumed.
type SessionResumedMsg struct {
	Session session.Session
	Err     error
}

// SessionDeletedMsg is sent after session deletion.
type SessionDeletedMsg struct {
	SessionID string
	Err       error
}

// SessionRenamedMsg is sent after session rename.
type SessionRenamedMsg struct {
	SessionID  string
	NewSummary string
	Err        error
}

// SessionExportedMsg is sent after session export.
type SessionExportedMsg struct {
	SessionID string
	Path      string
	Err       error
}

// PreviewLoadedMsg contains parsed JSONL messages for preview.
type PreviewLoadedMsg struct {
	SessionID string
	Messages  []ParsedMessage
	Err       error
}

// LiveCaptureMsg contains PTY capture output.
type LiveCaptureMsg struct {
	SessionID string
	Content   string
}

// RunningSessionsMsg contains list of currently running session IDs.
type RunningSessionsMsg struct {
	RunningIDs map[string]bool
}

// ArchivedSessionsLoadedMsg is sent when archived sessions are loaded.
type ArchivedSessionsLoadedMsg struct {
	Sessions []session.Session
	Projects []session.Project
	Err      error
}

// SessionRestoredMsg is sent after restoring a session from archive.
type SessionRestoredMsg struct {
	SessionID string
	Err       error
}

// ParsedMessage represents a single message from JSONL.
type ParsedMessage struct {
	Type    string // "user", "assistant", "summary"
	Content string
}

// ConfirmAction represents a pending confirmation dialog.
type ConfirmAction int

const (
	ConfirmNone ConfirmAction = iota
	ConfirmDelete
	ConfirmArchive
	ConfirmRestore
)

// DialogResultMsg is sent when a dialog is dismissed.
type DialogResultMsg struct {
	Action    ConfirmAction
	Confirmed bool
	Input     string // for rename dialog
}

// PreviewMode defines the preview panel modes.
type PreviewMode int

const (
	PreviewLive PreviewMode = iota
	PreviewMessages
	PreviewMeta
)

func (p PreviewMode) String() string {
	switch p {
	case PreviewLive:
		return "Live"
	case PreviewMessages:
		return "Messages"
	case PreviewMeta:
		return "Meta"
	default:
		return "Live"
	}
}

func (p PreviewMode) Next() PreviewMode {
	return (p + 1) % 3
}
