package app

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/vladislav-k/tui-claude/internal/config"
	"github.com/vladislav-k/tui-claude/internal/ptymanager"
	"github.com/vladislav-k/tui-claude/internal/session"
)

// dbg is a temporary debug logger; writes to /tmp/tui-debug.log.
var dbg *log.Logger

func init() {
	f, err := os.OpenFile("/tmp/tui-debug.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		dbg = log.New(os.Stderr, "[DBG] ", log.Ltime|log.Lmicroseconds)
	} else {
		dbg = log.New(f, "", log.Ltime|log.Lmicroseconds)
	}
}

// State represents the current UI state.
type State int

const (
	StateNormal State = iota
	StateSearching
	StateHelp
	StateStats
	StateConfirmDialog
	StateRenameDialog
)

// Model is the root Bubble Tea model.
type Model struct {
	cfg    config.Config
	keys   KeyMap
	state  State
	width  int
	height int
	ptyMgr *ptymanager.Manager

	// Data
	allSessions []session.Session
	sessions    []session.Session // filtered/sorted view
	projects    []session.Project
	runningIDs  map[string]bool
	sortField   session.SortField

	// UI components
	list        list.Model
	preview     viewport.Model
	searchInput textinput.Model

	// Tab state
	activeTab     int           // 0 = All, 1+ = project index
	allMode       bool          // true = show all sessions, false = current dir only
	sessionFilter SessionFilter // Active / Inactive / Archived
	previewMode   PreviewMode

	// Archived sessions
	archivedSessions []session.Session
	archivedProjects []session.Project

	// Dialog state
	confirmAction ConfirmAction
	renameInput   textinput.Model

	// Preview cache
	previewContent string
	previewSession string

	// Error display
	lastError string

	// Embedded terminal
	focusPanel        FocusPanel
	attachedSessionID string
	ptyOutputCh       chan struct{}
}

// NewModel creates a new app model.
func NewModel(cfg config.Config, ptyMgr *ptymanager.Manager) Model {
	// Search input
	si := textinput.New()
	si.Placeholder = "Search sessions..."
	si.Prompt = "/ "
	si.PromptStyle = lipgloss.NewStyle().Foreground(ColorCyan)
	si.TextStyle = lipgloss.NewStyle().Foreground(ColorTextBright)

	// Rename input
	ri := textinput.New()
	ri.Placeholder = "New summary..."
	ri.Prompt = "> "
	ri.PromptStyle = lipgloss.NewStyle().Foreground(ColorCyan)
	ri.TextStyle = lipgloss.NewStyle().Foreground(ColorTextBright)

	// List delegate
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(ColorCyan).
		BorderLeftForeground(ColorCyan)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(ColorTextNormal).
		BorderLeftForeground(ColorCyan)
	delegate.Styles.NormalTitle = delegate.Styles.NormalTitle.
		Foreground(ColorTextBright)
	delegate.Styles.NormalDesc = delegate.Styles.NormalDesc.
		Foreground(ColorTextMuted)
	delegate.Styles.DimmedTitle = delegate.Styles.DimmedTitle.
		Foreground(ColorTextMuted)
	delegate.Styles.DimmedDesc = delegate.Styles.DimmedDesc.
		Foreground(ColorTextMuted)

	l := list.New([]list.Item{}, delegate, 0, 0)
	l.Title = "Sessions"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()

	vp := viewport.New(0, 0)

	return Model{
		cfg:         cfg,
		keys:        DefaultKeyMap(),
		state:       StateNormal,
		ptyMgr:      ptyMgr,
		allMode:     cfg.AllMode,
		runningIDs:  make(map[string]bool),
		list:        l,
		preview:     vp,
		searchInput: si,
		renameInput: ri,
		previewMode: PreviewMessages,
	}
}

// Init starts the program by loading sessions.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		loadSessionsCmd(m.cfg.ClaudeDir),
		tickCmd(2*time.Second),
		m.detectRunningCmd(),
	)
}

func loadSessionsCmd(claudeDir string) tea.Cmd {
	return func() tea.Msg {
		sessions, projects, err := session.LoadAll(claudeDir)
		return SessionsLoadedMsg{
			Sessions: sessions,
			Projects: projects,
			Err:      err,
		}
	}
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateLayout()
		// Sync VT emulator and PTY size
		if m.attachedSessionID != "" {
			m.ptyMgr.ResizeEmulator(m.attachedSessionID, m.preview.Width, m.preview.Height)
			m.ptyMgr.ResizePTY(m.attachedSessionID, m.preview.Width, m.preview.Height)
		} else if sel, ok := m.list.SelectedItem().(sessionItem); ok {
			m.ptyMgr.ResizeEmulator(sel.session.SessionID, m.preview.Width, m.preview.Height)
		}
		return m, nil

	case SessionsLoadedMsg:
		if msg.Err != nil {
			m.lastError = msg.Err.Error()
			return m, nil
		}
		dbg.Printf("SessionsLoadedMsg: %d sessions from index, runningIDs=%v", len(msg.Sessions), m.runningIDs)
		for _, s := range msg.Sessions {
			if m.ptyMgr.IsRunning(s.SessionID) {
				dbg.Printf(
					"  index session (running): id=%s path=%q summary=%q fp=%q mc=%d",
					s.SessionID,
					s.FullPath,
					s.Summary,
					s.FirstPrompt[:min(len(s.FirstPrompt), 40)],
					s.MsgCount,
				)
			}
		}
		// Preserve running sessions absent from the index. This covers both
		// synthetic "new-*" entries and rekeyed sessions whose JSONL exists
		// on disk but hasn't been added to sessions-index.json yet.
		newSessions := msg.Sessions
		indexIDs := make(map[string]bool, len(newSessions))
		for _, ns := range newSessions {
			indexIDs[ns.SessionID] = true
		}
		for _, old := range m.allSessions {
			if !m.ptyMgr.IsRunning(old.SessionID) {
				continue
			}
			if !indexIDs[old.SessionID] {
				newSessions = append(newSessions, old)
			}
		}
		// Preserve enriched metadata for running sessions so that a stale
		// sessions-index.json doesn't overwrite fresh JSONL-based data.
		enriched := make(map[string]session.Session)
		for _, old := range m.allSessions {
			if m.ptyMgr.IsRunning(old.SessionID) {
				enriched[old.SessionID] = old
			}
		}
		for i, ns := range newSessions {
			if old, ok := enriched[ns.SessionID]; ok {
				if old.Summary != "" && old.Summary != "New session" {
					newSessions[i].Summary = old.Summary
				}
				if old.FirstPrompt != "" {
					newSessions[i].FirstPrompt = old.FirstPrompt
				}
				if old.MsgCount > newSessions[i].MsgCount {
					newSessions[i].MsgCount = old.MsgCount
				}
			}
		}
		m.allSessions = newSessions
		m.projects = msg.Projects
		m.rekeyNewSessions()
		// Refresh running IDs after rekeying so that applyFilters and
		// enrichRunningCmd see the real session UUIDs, not stale "new-*" keys.
		m.runningIDs = m.ptyMgr.DetectRunning()
		dbg.Printf("after rekey: runningIDs=%v", m.runningIDs)
		for _, s := range m.allSessions {
			if m.runningIDs[s.SessionID] {
				dbg.Printf(
					"  will enrich? id=%s path=%q summary=%q mc=%d",
					s.SessionID,
					s.FullPath,
					s.Summary,
					s.MsgCount,
				)
			}
		}
		m.applyFilters()
		return m, m.enrichRunningCmd()

	case RunningSessionsMsg:
		// Detect sessions that just stopped — run one final enrichment
		// so we pick up the summary entry Claude writes at session end.
		var justStopped []struct{ id, path string }
		for id := range m.runningIDs {
			if !msg.RunningIDs[id] {
				for _, s := range m.allSessions {
					if s.SessionID == id && s.FullPath != "" {
						justStopped = append(justStopped, struct{ id, path string }{id, s.FullPath})
						break
					}
				}
			}
		}
		m.runningIDs = msg.RunningIDs
		m.applyFilters()
		if len(justStopped) > 0 {
			return m, func() tea.Msg {
				results := make(enrichedSessionsMsg)
				for _, r := range justStopped {
					firstPrompt, summary, msgCount := session.ExtractMeta(r.path)
					results[r.id] = enrichedMeta{
						firstPrompt: firstPrompt,
						summary:     summary,
						msgCount:    msgCount,
					}
				}
				return results
			}
		}
		return m, nil

	case enrichedSessionsMsg:
		dbg.Printf("enrichedSessionsMsg: %d entries", len(msg))
		for id, meta := range msg {
			dbg.Printf(
				"  enriched: id=%s summary=%q fp=%q mc=%d",
				id,
				meta.summary,
				meta.firstPrompt[:min(len(meta.firstPrompt), 40)],
				meta.msgCount,
			)
		}
		for i := range m.allSessions {
			if meta, ok := msg[m.allSessions[i].SessionID]; ok {
				if meta.summary != "" {
					m.allSessions[i].Summary = meta.summary
				}
				if meta.firstPrompt != "" {
					m.allSessions[i].FirstPrompt = meta.firstPrompt
				}
				m.allSessions[i].MsgCount = meta.msgCount
			}
		}
		m.applyFilters()
		return m, nil

	case PreviewLoadedMsg:
		if msg.Err != nil {
			m.previewContent = "Could not parse session data"
		} else {
			m.previewSession = msg.SessionID
			m.previewContent = renderMessages(msg.Messages, m.preview.Width)
		}
		m.preview.SetContent(m.previewContent)
		m.preview.GotoTop()
		return m, nil

	case LiveCaptureMsg:
		m.previewContent = msg.Content
		m.preview.SetContent(m.previewContent)
		return m, nil

	case embeddedAttachMsg:
		// Detach previous session if any
		if m.attachedSessionID != "" {
			m.ptyMgr.SetForward(m.attachedSessionID, nil)
			if m.ptyOutputCh != nil {
				close(m.ptyOutputCh)
			}
		}
		m.attachedSessionID = msg.sessionID
		m.focusPanel = FocusRight
		m.previewMode = PreviewLive
		m.sessionFilter = FilterActive
		// Immediately mark as running so it appears in Active filter
		if m.runningIDs == nil {
			m.runningIDs = make(map[string]bool)
		}
		m.runningIDs[msg.sessionID] = true
		// For new sessions not yet on disk, add a synthetic entry
		found := false
		for _, s := range m.allSessions {
			if s.SessionID == msg.sessionID {
				found = true
				break
			}
		}
		if !found {
			m.allSessions = append(
				m.allSessions, session.Session{
					SessionID:   msg.sessionID,
					ProjectPath: msg.projectPath,
					ProjectName: filepath.Base(msg.projectPath),
					Summary:     "New session",
					Modified:    time.Now(),
					IsRunning:   true,
				},
			)
		}
		m.applyFilters()
		// Resize VT emulator and PTY to match preview panel
		m.ptyMgr.ResizeEmulator(msg.sessionID, m.preview.Width, m.preview.Height)
		m.ptyMgr.ResizePTY(msg.sessionID, m.preview.Width, m.preview.Height)
		// Set up PTY output notification channel
		m.ptyOutputCh = make(chan struct{}, 1)
		m.ptyMgr.SetForward(msg.sessionID, &chanNotifyWriter{ch: m.ptyOutputCh})
		// Initial render
		content := m.ptyMgr.Capture(msg.sessionID)
		m.previewContent = content
		m.preview.SetContent(content)
		m.preview.GotoBottom()
		return m, tea.Batch(
			listenForPTYOutput(m.ptyOutputCh),
			m.detectRunningCmd(),
		)

	case PTYOutputMsg:
		if m.attachedSessionID != "" {
			content := m.ptyMgr.Capture(m.attachedSessionID)
			m.previewContent = content
			m.preview.SetContent(content)
			m.preview.GotoBottom()
			if m.ptyOutputCh != nil {
				return m, listenForPTYOutput(m.ptyOutputCh)
			}
		}
		return m, nil

	case DialogResultMsg:
		return m.handleDialogResult(msg)

	case SessionDeletedMsg:
		if msg.Err != nil {
			m.lastError = msg.Err.Error()
		} else {
			if m.sessionFilter == FilterArchived {
				m.removeArchivedSession(msg.SessionID)
			} else {
				m.removeSession(msg.SessionID)
			}
		}
		m.state = StateNormal
		return m, nil

	case SessionResumedMsg:
		if msg.Err != nil {
			m.lastError = msg.Err.Error()
		}
		// Reload sessions from disk so newly created sessions appear in the list
		return m, tea.Batch(
			loadSessionsCmd(m.cfg.ClaudeDir),
			m.detectRunningCmd(),
		)

	case SessionExportedMsg:
		if msg.Err != nil {
			m.lastError = msg.Err.Error()
		} else {
			m.lastError = "" // clear
		}
		return m, nil

	case ArchivedSessionsLoadedMsg:
		if msg.Err != nil {
			m.lastError = msg.Err.Error()
			return m, nil
		}
		m.archivedSessions = msg.Sessions
		m.archivedProjects = msg.Projects
		m.applyFilters()
		return m, nil

	case SessionRestoredMsg:
		if msg.Err != nil {
			m.lastError = msg.Err.Error()
		} else {
			// Reload both collections
			return m, tea.Batch(
				loadSessionsCmd(m.cfg.ClaudeDir),
				m.loadArchivedSessionsCmd(),
			)
		}
		return m, nil

	case SessionRenamedMsg:
		if msg.Err != nil {
			m.lastError = msg.Err.Error()
		} else {
			m.updateSessionSummary(msg.SessionID, msg.NewSummary)
		}
		m.state = StateNormal
		return m, nil

	case tickMsg:
		// Periodic refresh: detect running sessions, refresh live preview
		cmds := []tea.Cmd{
			m.detectRunningCmd(),
			tickCmd(2 * time.Second),
			loadSessionsCmd(m.cfg.ClaudeDir),
		}
		// Auto-detach if attached session is no longer running
		if m.attachedSessionID != "" && !m.ptyMgr.IsRunning(m.attachedSessionID) {
			m.ptyMgr.SetForward(m.attachedSessionID, nil)
			if m.ptyOutputCh != nil {
				close(m.ptyOutputCh)
				m.ptyOutputCh = nil
			}
			m.focusPanel = FocusLeft
			m.attachedSessionID = ""
		}
		if m.previewMode == PreviewLive && m.attachedSessionID == "" {
			if sel, ok := m.list.SelectedItem().(sessionItem); ok {
				cmds = append(cmds, m.liveCaptureCmd(sel.session))
			}
		}
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Forward to active sub-component
	switch m.state {
	case StateNormal:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		cmds = append(cmds, cmd)

		// Update preview on cursor change
		if sel, ok := m.list.SelectedItem().(sessionItem); ok {
			if sel.session.SessionID != m.previewSession {
				cmds = append(cmds, m.loadPreviewCmd(sel.session))
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Embedded terminal mode: forward all keys to PTY
	if m.focusPanel == FocusRight && m.attachedSessionID != "" {
		return m.handleTerminalKey(msg)
	}

	// Global keys
	if key.Matches(msg, m.keys.Quit) && m.state == StateNormal {
		return m, tea.Quit
	}

	if key.Matches(msg, m.keys.Escape) {
		switch m.state {
		case StateSearching:
			m.state = StateNormal
			m.list.SetFilteringEnabled(true)
			return m, nil
		case StateHelp, StateStats, StateConfirmDialog, StateRenameDialog:
			m.state = StateNormal
			return m, nil
		}
	}

	switch m.state {
	case StateNormal:
		return m.handleNormalKey(msg)
	case StateSearching:
		return m.handleSearchKey(msg)
	case StateRenameDialog:
		return m.handleRenameKey(msg)
	case StateConfirmDialog:
		return m.handleConfirmKey(msg)
	}

	return m, nil
}

func (m Model) handleTerminalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Ctrl+] — detach from embedded terminal
	if key.Matches(msg, m.keys.Detach) || msg.Type == tea.KeyCtrlCloseBracket {
		return m.detachTerminal()
	}

	// Forward all other keys to PTY
	raw := ptymanager.KeyMsgToBytes(msg)
	if raw != nil {
		m.ptyMgr.WriteToPTY(m.attachedSessionID, raw)
	}
	return m, nil
}

// detachTerminal detaches from embedded terminal and returns focus to list.
func (m Model) detachTerminal() (tea.Model, tea.Cmd) {
	m.ptyMgr.SetForward(m.attachedSessionID, nil)
	if m.ptyOutputCh != nil {
		close(m.ptyOutputCh)
		m.ptyOutputCh = nil
	}
	m.focusPanel = FocusLeft
	m.attachedSessionID = ""
	return m, tea.Batch(
		loadSessionsCmd(m.cfg.ClaudeDir),
		m.detectRunningCmd(),
	)
}

func (m Model) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Help):
		m.state = StateHelp
		return m, nil

	case key.Matches(msg, m.keys.Stats):
		m.state = StateStats
		return m, nil

	case key.Matches(msg, m.keys.Search):
		m.state = StateSearching
		m.searchInput.Reset()
		m.searchInput.Focus()
		return m, textinput.Blink

	case key.Matches(msg, m.keys.Sort):
		m.sortField = m.sortField.Next()
		m.applyFilters()
		return m, nil

	case key.Matches(msg, m.keys.ToggleArchive):
		m.sessionFilter = m.sessionFilter.Next()
		m.activeTab = 0
		if m.sessionFilter == FilterArchived && len(m.archivedSessions) == 0 {
			m.applyFilters()
			return m, m.loadArchivedSessionsCmd()
		}
		m.applyFilters()
		return m, nil

	case key.Matches(msg, m.keys.Refresh):
		if m.sessionFilter == FilterArchived {
			return m, tea.Batch(
				loadSessionsCmd(m.cfg.ClaudeDir),
				m.loadArchivedSessionsCmd(),
			)
		}
		return m, loadSessionsCmd(m.cfg.ClaudeDir)

	case key.Matches(msg, m.keys.TabSwitch):
		m.previewMode = m.previewMode.Next()
		if sel, ok := m.list.SelectedItem().(sessionItem); ok {
			return m, m.loadPreviewCmd(sel.session)
		}
		return m, nil

	case key.Matches(msg, m.keys.ToggleAll):
		m.allMode = !m.allMode
		m.activeTab = 0
		m.applyFilters()
		return m, nil

	case key.Matches(msg, m.keys.TabLeft):
		if m.allMode && m.activeTab > 0 {
			m.activeTab--
			m.applyFilters()
		}
		return m, nil

	case key.Matches(msg, m.keys.TabRight):
		if m.allMode && m.activeTab < len(m.projects) {
			m.activeTab++
			m.applyFilters()
		}
		return m, nil

	case key.Matches(msg, m.keys.Space):
		if sel, ok := m.list.SelectedItem().(sessionItem); ok {
			m.toggleSelection(sel.session.SessionID)
			m.applyFilters()
		}
		return m, nil

	case key.Matches(msg, m.keys.Delete):
		if sel, ok := m.list.SelectedItem().(sessionItem); ok {
			m.state = StateConfirmDialog
			m.confirmAction = ConfirmDelete
			_ = sel // we'll use current selection
		}
		return m, nil

	case key.Matches(msg, m.keys.Rename):
		if m.sessionFilter == FilterArchived {
			return m, nil
		}
		if sel, ok := m.list.SelectedItem().(sessionItem); ok {
			m.state = StateRenameDialog
			m.renameInput.Reset()
			m.renameInput.SetValue(sel.session.Summary)
			m.renameInput.Focus()
			return m, textinput.Blink
		}
		return m, nil

	case key.Matches(msg, m.keys.Enter):
		if m.sessionFilter == FilterArchived {
			return m, nil
		}
		if sel, ok := m.list.SelectedItem().(sessionItem); ok {
			return m, m.embedSessionCmd(sel.session)
		}
		return m, nil

	case key.Matches(msg, m.keys.Export):
		if sel, ok := m.list.SelectedItem().(sessionItem); ok {
			return m, m.exportSessionCmd(sel.session)
		}
		return m, nil

	case key.Matches(msg, m.keys.New):
		if m.sessionFilter == FilterArchived {
			return m, nil
		}
		return m, m.newSessionCmd()

	case key.Matches(msg, m.keys.Archive):
		if sel, ok := m.list.SelectedItem().(sessionItem); ok {
			if m.sessionFilter == FilterArchived {
				// In archive mode, A = restore
				m.state = StateConfirmDialog
				m.confirmAction = ConfirmRestore
			} else {
				m.state = StateConfirmDialog
				m.confirmAction = ConfirmArchive
			}
			_ = sel
		}
		return m, nil
	}

	// Forward to list (navigation keys like j/k)
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)

	// Check if selected item changed → load preview
	var cmds []tea.Cmd
	if cmd != nil {
		cmds = append(cmds, cmd)
	}
	if sel, ok := m.list.SelectedItem().(sessionItem); ok {
		if sel.session.SessionID != m.previewSession {
			cmds = append(cmds, m.loadPreviewCmd(sel.session))
		}
	}
	return m, tea.Batch(cmds...)
}

func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "enter" {
		m.state = StateNormal
		m.filterBySearch(m.searchInput.Value())
		return m, nil
	}

	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return m, cmd
}

func (m Model) handleRenameKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "enter" {
		if sel, ok := m.list.SelectedItem().(sessionItem); ok {
			m.state = StateNormal
			return m, m.renameSessionCmd(sel.session, m.renameInput.Value())
		}
		m.state = StateNormal
		return m, nil
	}

	var cmd tea.Cmd
	m.renameInput, cmd = m.renameInput.Update(msg)
	return m, cmd
}

func (m Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		if sel, ok := m.list.SelectedItem().(sessionItem); ok {
			m.state = StateNormal
			switch m.confirmAction {
			case ConfirmDelete:
				return m, m.deleteSessionCmd(sel.session)
			case ConfirmArchive:
				return m, m.archiveSessionCmd(sel.session)
			case ConfirmRestore:
				return m, m.restoreSessionCmd(sel.session)
			}
		}
		m.state = StateNormal
	case "n", "N", "esc":
		m.state = StateNormal
	}
	return m, nil
}

func (m Model) handleDialogResult(msg DialogResultMsg) (tea.Model, tea.Cmd) {
	m.state = StateNormal
	return m, nil
}

// View renders the UI.
func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	switch m.state {
	case StateHelp:
		return m.renderHelp()
	case StateStats:
		return m.renderStats()
	}

	// Main layout
	tabBar := m.renderTabBar()
	content := m.renderContent()
	statusBar := m.renderStatusBar()

	// Overlay dialogs
	if m.state == StateConfirmDialog {
		content = m.renderConfirmOverlay(content)
	} else if m.state == StateRenameDialog {
		content = m.renderRenameOverlay(content)
	} else if m.state == StateSearching {
		content = m.renderSearchOverlay(content)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		tabBar,
		content,
		statusBar,
	)
}

func (m *Model) updateLayout() {
	tabBarHeight := 1
	statusBarHeight := 1
	contentHeight := m.height - tabBarHeight - statusBarHeight

	listWidth := m.width * 2 / 5
	if !m.cfg.PreviewEnabled {
		listWidth = m.width
	}

	previewWidth := m.width - listWidth

	// List: subtract panel borders (2 left+right) for inner content
	m.list.SetSize(listWidth-2, contentHeight-2)
	// Preview: subtract panel borders (2) + inner padding (2)
	m.preview.Width = previewWidth - 2 - 2
	// Preview: subtract top+bottom borders, title is in the top border
	m.preview.Height = contentHeight - 2
}

func (m *Model) applyFilters() {
	var filtered []session.Session
	var activeProjects []session.Project

	switch m.sessionFilter {
	case FilterActive:
		// Only sessions with a running PTY process
		var active []session.Session
		for _, s := range m.allSessions {
			if m.runningIDs[s.SessionID] {
				active = append(active, s)
			}
		}
		filtered = active
		activeProjects = m.projects
	case FilterInactive:
		// Non-archived sessions without a running process
		var inactive []session.Session
		for _, s := range m.allSessions {
			if !m.runningIDs[s.SessionID] {
				inactive = append(inactive, s)
			}
		}
		filtered = inactive
		activeProjects = m.projects
	case FilterArchived:
		filtered = m.archivedSessions
		activeProjects = m.archivedProjects
	}

	// WorkDir filter (current directory mode)
	if !m.allMode && m.cfg.WorkDir != "" {
		filtered = session.FilterByWorkDir(filtered, m.cfg.WorkDir)
	}

	// Tab filter (only in allMode)
	if m.allMode && m.activeTab > 0 && m.activeTab <= len(activeProjects) {
		filtered = session.FilterByProject(filtered, activeProjects[m.activeTab-1].Path)
	}

	// Sort
	session.SortSessions(filtered, m.sortField)

	m.sessions = filtered

	// Update list items
	items := make([]list.Item, len(filtered))
	for i, s := range filtered {
		if m.sessionFilter != FilterArchived {
			s.IsRunning = m.runningIDs[s.SessionID]
		}
		items[i] = sessionItem{session: s}
	}
	m.list.SetItems(items)
}

func (m *Model) filterBySearch(query string) {
	if query == "" {
		m.applyFilters()
		return
	}
	m.list.SetFilteringEnabled(true)
}

func (m *Model) toggleSelection(id string) {
	for i := range m.allSessions {
		if m.allSessions[i].SessionID == id {
			m.allSessions[i].Selected = !m.allSessions[i].Selected
			break
		}
	}
}

func (m *Model) removeSession(id string) {
	var remaining []session.Session
	for _, s := range m.allSessions {
		if s.SessionID != id {
			remaining = append(remaining, s)
		}
	}
	m.allSessions = remaining
	m.applyFilters()
}

func (m *Model) removeArchivedSession(id string) {
	var remaining []session.Session
	for _, s := range m.archivedSessions {
		if s.SessionID != id {
			remaining = append(remaining, s)
		}
	}
	m.archivedSessions = remaining
	m.applyFilters()
}

// rekeyNewSessions matches PTY sessions with synthetic "new-*" IDs
// to real session UUIDs loaded from disk, so they appear correctly
// in the Active filter.
func (m *Model) rekeyNewSessions() {
	pending := m.ptyMgr.RunningNewSessions()
	if len(pending) == 0 {
		return
	}

	// Build set of known session IDs so we skip them during filesystem scan.
	known := make(map[string]bool, len(m.allSessions))
	for _, s := range m.allSessions {
		known[s.SessionID] = true
	}

	for newID, projPath := range pending {
		// Extract creation time from "new-<unixnano>" ID
		var createdNano int64
		fmt.Sscanf(newID, "new-%d", &createdNano)
		createdTime := time.Unix(0, createdNano)

		// Find the newest disk session matching this project path
		// that was modified AFTER the PTY was created and isn't already tracked
		var bestID string
		var bestPath string
		var bestTime time.Time
		for _, s := range m.allSessions {
			if s.ProjectPath != projPath {
				continue
			}
			if m.ptyMgr.IsRunning(s.SessionID) {
				continue // already tracked under real ID
			}
			if len(s.SessionID) > 4 && s.SessionID[:4] == "new-" {
				continue // skip synthetic entries
			}
			if !s.Modified.After(createdTime.Add(-5 * time.Second)) {
				continue // only match sessions created after PTY launch
			}
			if s.Modified.After(bestTime) {
				bestTime = s.Modified
				bestID = s.SessionID
				bestPath = s.FullPath
			}
		}

		// Fallback: scan the project directory for JSONL files not in the
		// index. Claude CLI may not update sessions-index.json while a
		// session is active, so the real JSONL file exists on disk but is
		// absent from m.allSessions.
		if bestID == "" {
			bestID, bestPath, bestTime = m.scanForNewSession(projPath, createdTime, known)
		}

		if bestID != "" {
			dbg.Printf("rekey: %s -> %s (path=%s)", newID, bestID, bestPath)
			m.ptyMgr.RekeySession(newID, bestID)
			// Update attachedSessionID if this was the embedded session
			if m.attachedSessionID == newID {
				m.attachedSessionID = bestID
			}
			// If the real session wasn't in allSessions (found via scan),
			// add it so enrichRunningCmd can read its JSONL.
			if !known[bestID] {
				m.allSessions = append(
					m.allSessions, session.Session{
						SessionID:   bestID,
						FullPath:    bestPath,
						Modified:    bestTime,
						Created:     createdTime,
						ProjectPath: projPath,
						ProjectName: filepath.Base(projPath),
						Summary:     "New session",
					},
				)
				known[bestID] = true
			}
			// Remove the synthetic entry now that we have the real one
			for i, s := range m.allSessions {
				if s.SessionID == newID {
					m.allSessions = append(m.allSessions[:i], m.allSessions[i+1:]...)
					break
				}
			}
		}
	}
}

// scanForNewSession scans the project directory on disk for a JSONL file
// that appeared after createdTime and is not already tracked in known.
func (m *Model) scanForNewSession(projPath string, createdTime time.Time, known map[string]bool) (id, fullPath string, modTime time.Time) {
	// Encode project path to directory name (Claude CLI convention).
	encoded := strings.NewReplacer("/", "-", ".", "-").Replace(projPath)
	projDir := filepath.Join(m.cfg.ClaudeDir, encoded)

	entries, err := os.ReadDir(projDir)
	if err != nil {
		return "", "", time.Time{}
	}

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		sessionID := strings.TrimSuffix(name, ".jsonl")
		if known[sessionID] {
			continue
		}
		if m.ptyMgr.IsRunning(sessionID) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if !info.ModTime().After(createdTime.Add(-5 * time.Second)) {
			continue
		}
		if info.ModTime().After(modTime) {
			modTime = info.ModTime()
			id = sessionID
			fullPath = filepath.Join(projDir, name)
		}
	}
	return id, fullPath, modTime
}

func (m *Model) updateSessionSummary(id, summary string) {
	for i := range m.allSessions {
		if m.allSessions[i].SessionID == id {
			m.allSessions[i].Summary = summary
			break
		}
	}
	m.applyFilters()
}

// sessionItem implements list.Item for the bubbles list.
type sessionItem struct {
	session session.Session
}

func (i sessionItem) Title() string {
	prefix := "  "
	if i.session.IsRunning {
		prefix = Styles.RunningDot.Render("●") + " "
	}
	if i.session.Selected {
		prefix = Styles.SelectedMark.Render("▸") + " "
	}
	return prefix + i.session.DisplayTitle()
}

func (i sessionItem) Description() string {
	return i.session.ProjectName + " · " + i.session.Age() + " · " + itoa(i.session.MsgCount) + " msgs"
}

func (i sessionItem) FilterValue() string {
	return i.session.DisplayTitle() + " " + i.session.FirstPrompt + " " + i.session.ProjectName
}

func itoa(i int) string {
	if i < 0 {
		return "-" + uitoa(-i)
	}
	return uitoa(i)
}

func uitoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return uitoa(i/10) + string(rune('0'+i%10))
}

// Render methods

func (m Model) renderTabBar() string {
	titleIcon := Styles.PanelTitle.Render("◆")
	title := Styles.Title.Render("tui-claude")

	// Filter badge
	var badge string
	switch m.sessionFilter {
	case FilterActive:
		badge = Styles.ActiveBadge.Render("ACTIVE")
	case FilterInactive:
		badge = Styles.InactiveBadge.Render("INACTIVE")
	case FilterArchived:
		badge = Styles.ArchiveBadge.Render("ARCHIVED")
	}

	// Determine project list for tabs
	var tabProjects []session.Project
	switch m.sessionFilter {
	case FilterArchived:
		tabProjects = m.archivedProjects
	default:
		tabProjects = m.projects
	}

	if !m.allMode {
		dirName := filepath.Base(m.cfg.WorkDir)
		dirLabel := Styles.TabActive.Render("@ " + dirName)
		bar := lipgloss.JoinHorizontal(lipgloss.Center, titleIcon, " ", title, "  ", badge, "  ", dirLabel)
		return Styles.TabBar.Width(m.width).Render(bar)
	}

	tabs := []string{"All"}
	for _, p := range tabProjects {
		tabs = append(tabs, p.Name)
	}

	var rendered []string
	for i, t := range tabs {
		if i == m.activeTab {
			rendered = append(rendered, Styles.TabActive.Render(t))
		} else {
			rendered = append(rendered, Styles.TabInactive.Render(t))
		}
	}

	tabLine := lipgloss.JoinHorizontal(lipgloss.Center, rendered...)
	bar := lipgloss.JoinHorizontal(lipgloss.Center, titleIcon, " ", title, "  ", badge, "  ", tabLine)
	return Styles.TabBar.Width(m.width).Render(bar)
}

func (m Model) renderContent() string {
	listWidth := m.width * 2 / 5
	if !m.cfg.PreviewEnabled {
		listWidth = m.width
	}
	contentHeight := m.height - 2 // tabBar + statusBar

	listView := m.list.View()

	// Show hint when no sessions
	if len(m.sessions) == 0 {
		var hint string
		switch m.sessionFilter {
		case FilterActive:
			hint = Styles.HelpDesc.Render("No running sessions.\nPress " + Styles.HelpKey.Render("V") + " for inactive.")
		case FilterInactive:
			hint = Styles.HelpDesc.Render("No inactive sessions.\nPress " + Styles.HelpKey.Render("V") + " for archived.")
		case FilterArchived:
			hint = Styles.HelpDesc.Render("No archived sessions.\nPress " + Styles.HelpKey.Render("V") + " for active.")
		}
		if !m.allMode && len(m.allSessions) > 0 && hint == "" {
			hint = Styles.HelpDesc.Render("No sessions for current directory.\nPress " + Styles.HelpKey.Render("a") + " to show all projects.")
		}
		if hint != "" {
			listView = lipgloss.Place(listWidth-2, contentHeight-2, lipgloss.Center, lipgloss.Center, hint)
		}
	}

	// List panel title — highlight border based on focus
	listTitle := "Sessions (" + itoa(len(m.sessions)) + ")"
	listBorderColor := ColorBorderDim
	listTitleColor := ColorCyan
	if m.focusPanel == FocusLeft || m.attachedSessionID == "" {
		listBorderColor = ColorBorderActive
	}
	listPanel := renderTitledPanel(listTitle, listView, listWidth, contentHeight, listBorderColor, listTitleColor)

	if !m.cfg.PreviewEnabled {
		return listPanel
	}

	// Preview panel — highlight border when terminal has focus
	previewWidth := m.width - listWidth
	previewContent := m.preview.View()
	previewTitle := m.previewTitleLine()
	previewBorderColor := ColorBorderDim
	previewTitleColor := ColorCyan
	if m.focusPanel == FocusRight && m.attachedSessionID != "" {
		previewBorderColor = ColorBorderActive
	}
	previewPanel := renderTitledPanel(
		previewTitle,
		previewContent,
		previewWidth,
		contentHeight,
		previewBorderColor,
		previewTitleColor,
	)

	return lipgloss.JoinHorizontal(lipgloss.Top, listPanel, previewPanel)
}

func (m Model) previewTitleLine() string {
	modes := []PreviewMode{PreviewLive, PreviewMessages, PreviewMeta}
	var parts []string
	for _, mode := range modes {
		if mode == m.previewMode {
			parts = append(parts, Styles.PreviewTabAct.Render(mode.String()))
		} else {
			parts = append(parts, Styles.PreviewTab.Render(mode.String()))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, parts...)
}

// renderTitledPanel creates a panel with a title embedded in the top border.
func renderTitledPanel(title, content string, width, height int, borderColor, titleColor lipgloss.Color) string {
	if width < 4 {
		return content
	}

	innerWidth := width - 2 // left + right border chars

	// Style for border characters
	borderStyle := lipgloss.NewStyle().Foreground(borderColor)
	titleStyle := lipgloss.NewStyle().Foreground(titleColor).Bold(true)

	// Top line: ╭─ Title ─────╮
	titleText := titleStyle.Render(title)
	titleVisualWidth := lipgloss.Width(titleText)
	topFillWidth := innerWidth - 2 - titleVisualWidth // 2 = "─ " before title + " " after
	if topFillWidth < 0 {
		topFillWidth = 0
	}
	topFill := ""
	for i := 0; i < topFillWidth; i++ {
		topFill += "─"
	}
	topLine := borderStyle.Render("╭─") + " " + titleText + " " + borderStyle.Render(topFill+"╮")

	// Bottom line: ╰──────────╯
	bottomFill := ""
	for i := 0; i < innerWidth; i++ {
		bottomFill += "─"
	}
	bottomLine := borderStyle.Render("╰" + bottomFill + "╯")

	// Side borders for content
	leftBorder := borderStyle.Render("│")
	rightBorder := borderStyle.Render("│")

	// Wrap content lines with side borders
	contentLines := strings.Split(content, "\n")
	innerHeight := height - 2 // top + bottom border
	if innerHeight < 0 {
		innerHeight = 0
	}

	// Pad or truncate content to fill the panel height
	for len(contentLines) < innerHeight {
		contentLines = append(contentLines, "")
	}
	if len(contentLines) > innerHeight {
		contentLines = contentLines[:innerHeight]
	}

	var body strings.Builder
	for _, line := range contentLines {
		lineWidth := lipgloss.Width(line)
		pad := innerWidth - lineWidth
		if pad < 0 {
			pad = 0
		}
		padding := strings.Repeat(" ", pad)
		body.WriteString(leftBorder + line + padding + rightBorder + "\n")
	}

	return topLine + "\n" + body.String() + bottomLine
}

// renderKeyHint formats a key hint for the status bar.
func renderKeyHint(key, desc string) string {
	return Styles.HelpKey.Render(key) + " " + Styles.HelpDesc.Render(desc)
}

func (m Model) renderStatusBar() string {
	sessionCount := itoa(len(m.sessions))
	sort := m.sortField.String()

	var projectCount string
	if m.sessionFilter == FilterArchived {
		projectCount = itoa(len(m.archivedProjects))
	} else {
		projectCount = itoa(len(m.projects))
	}

	mode := "Current dir"
	if m.allMode {
		mode = "All"
	}

	sep := Styles.StatusSep.Render(" │ ")

	left := Styles.StatusVal.Render(sessionCount) + " sessions" + sep +
		Styles.StatusVal.Render(projectCount) + " projects" + sep +
		"Sort: " + Styles.StatusVal.Render(sort) + sep + mode

	// Badge on the left
	switch m.sessionFilter {
	case FilterActive:
		left = Styles.ActiveBadge.Render("ACTIVE") + "  " + left
	case FilterInactive:
		left = Styles.InactiveBadge.Render("INACTIVE") + "  " + left
	case FilterArchived:
		left = Styles.ArchiveBadge.Render("ARCHIVED") + "  " + left
	}

	// Key hints on the right — adapt to mode
	var right string
	if m.focusPanel == FocusRight && m.attachedSessionID != "" {
		right = renderKeyHint("Ctrl+]", "detach") + sep +
			renderKeyHint("", "typing goes to Claude")
	} else if m.sessionFilter == FilterArchived {
		right = renderKeyHint("A", "restore") + sep +
			renderKeyHint("d", "delete") + sep +
			renderKeyHint("e", "export") + sep +
			renderKeyHint("V", "next") + sep +
			renderKeyHint("?", "help")
	} else {
		right = renderKeyHint("q", "quit") + sep +
			renderKeyHint("/", "search") + sep +
			renderKeyHint("a", "toggle") + sep +
			renderKeyHint("V", "next") + sep +
			renderKeyHint("?", "help")
	}

	if m.lastError != "" {
		left = Styles.Error.Render("Error: " + m.lastError)
	}

	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	gap := m.width - leftWidth - rightWidth - 2
	if gap < 0 {
		gap = 0
	}

	return Styles.StatusBar.Width(m.width).Render(left + strings.Repeat(" ", gap) + right)
}

func (m Model) renderConfirmOverlay(base string) string {
	title := "Delete session?"
	switch m.confirmAction {
	case ConfirmArchive:
		title = "Archive session?"
	case ConfirmRestore:
		title = "Restore session from archive?"
	}
	dialog := Styles.DialogTitle.Render(title) + "\n\n" +
		"Press " + Styles.ConfirmYes.Render("y") + " to confirm, " +
		Styles.ConfirmNo.Render("n") + " to cancel"
	box := Styles.Dialog.Render(dialog)
	return placeOverlay(m.width, m.height-2, box, base)
}

func (m Model) renderRenameOverlay(base string) string {
	dialog := Styles.DialogTitle.Render("Rename session") + "\n\n" +
		m.renameInput.View() + "\n\n" +
		Styles.HelpDesc.Render("Enter to confirm, Esc to cancel")
	box := Styles.Dialog.Render(dialog)
	return placeOverlay(m.width, m.height-2, box, base)
}

func (m Model) renderSearchOverlay(base string) string {
	searchBox := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ColorCyan).
		Background(ColorBgSurface).
		Padding(0, 1).
		Width(40).
		Render(m.searchInput.View())
	return placeOverlay(m.width, m.height-2, searchBox, base)
}

func (m Model) renderHelp() string {
	var bindings []struct{ key, desc string }

	if m.sessionFilter == FilterArchived {
		bindings = []struct{ key, desc string }{
			{"↑/k, ↓/j", "Navigate list"},
			{"A", "Restore session"},
			{"d", "Delete session"},
			{"e", "Export to markdown"},
			{"/", "Search sessions"},
			{"s", "Cycle sort (date/project/messages)"},
			{"a", "Toggle all projects / current dir"},
			{"Tab", "Switch preview mode"},
			{"h/l", "Switch project tabs (all mode)"},
			{"V", "Cycle filter (active/inactive/archived)"},
			{"?", "This help"},
			{"R", "Refresh list"},
			{"q", "Quit"},
		}
	} else {
		bindings = []struct{ key, desc string }{
			{"↑/k, ↓/j", "Navigate list"},
			{"Enter", "Open session in panel"},
			{"Ctrl+]", "Detach from terminal"},
			{"n", "New Claude session"},
			{"d", "Delete session"},
			{"r", "Rename (edit summary)"},
			{"/", "Search sessions"},
			{"s", "Cycle sort (date/project/messages)"},
			{"a", "Toggle all projects / current dir"},
			{"Tab", "Switch preview mode"},
			{"Space", "Multi-select"},
			{"e", "Export to markdown"},
			{"A", "Archive session"},
			{"V", "Cycle filter (active/inactive/archived)"},
			{"h/l", "Switch project tabs (all mode)"},
			{"?", "This help"},
			{"S", "Statistics"},
			{"R", "Refresh list"},
			{"q", "Quit"},
		}
	}

	title := Styles.Title.Render("Keyboard Shortcuts") + "\n\n"
	var lines string
	for _, b := range bindings {
		lines += "  " + Styles.HelpKey.Width(14).Render(b.key) + Styles.HelpDesc.Render(b.desc) + "\n"
	}
	lines += "\n" + Styles.HelpDesc.Render("Press Esc or ? to close")

	content := title + lines
	box := Styles.Dialog.Width(50).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) renderStats() string {
	totalSessions := len(m.allSessions)
	totalProjects := len(m.projects)
	totalMessages := 0
	for _, s := range m.allSessions {
		totalMessages += s.MsgCount
	}

	title := Styles.Title.Render("Statistics") + "\n\n"
	stats := "  " + Styles.HelpKey.Width(20).Render("Total sessions:") + itoa(totalSessions) + "\n" +
		"  " + Styles.HelpKey.Width(20).Render("Total projects:") + itoa(totalProjects) + "\n" +
		"  " + Styles.HelpKey.Width(20).Render("Total messages:") + itoa(totalMessages) + "\n"

	if totalSessions > 0 {
		avg := totalMessages / totalSessions
		stats += "  " + Styles.HelpKey.Width(20).Render("Avg msgs/session:") + itoa(avg) + "\n"
	}

	stats += "\n"
	for _, p := range m.projects {
		stats += "  " + Styles.ProjectTag.Render(p.Name) + " " + itoa(len(p.Sessions)) + " sessions\n"
	}

	stats += "\n" + Styles.HelpDesc.Render("Press Esc or S to close")

	content := title + stats
	box := Styles.Dialog.Width(50).Render(content)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func renderMessages(msgs []ParsedMessage, width int) string {
	if width <= 0 {
		width = 80
	}

	renderer, _ := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
	)

	sepWidth := width
	if sepWidth > 40 {
		sepWidth = 40
	}
	separator := Styles.MsgSeparator.Render(strings.Repeat("─", sepWidth))

	var out strings.Builder
	for i, msg := range msgs {
		if i > 0 {
			out.WriteString(separator + "\n")
		}
		switch msg.Type {
		case "user":
			out.WriteString(Styles.UserLabel.Render("YOU") + "\n")
			out.WriteString(Styles.UserMsg.Render(msg.Content) + "\n\n")
		case "assistant":
			out.WriteString(Styles.AssistantLabel.Render("CLAUDE") + "\n")
			if renderer != nil {
				if rendered, err := renderer.Render(msg.Content); err == nil {
					out.WriteString(rendered + "\n")
				} else {
					out.WriteString(Styles.AssistantMsg.Render(msg.Content) + "\n\n")
				}
			} else {
				out.WriteString(Styles.AssistantMsg.Render(msg.Content) + "\n\n")
			}
		case "summary":
			out.WriteString(Styles.SummaryLabel.Render("SUMMARY") + "\n")
			out.WriteString(Styles.SummaryMsg.Render(msg.Content) + "\n\n")
		}
	}
	return out.String()
}

// placeOverlay places a dialog box centered over the base content.
func placeOverlay(width, height int, overlay, base string) string {
	return lipgloss.Place(
		width, height, lipgloss.Center, lipgloss.Center,
		overlay,
		lipgloss.WithWhitespaceChars(" "),
	)
}

// chanNotifyWriter is an io.Writer that sends a non-blocking signal to a
// buffered channel on each Write. Used to bridge PTY output to Bubble Tea.
type chanNotifyWriter struct {
	ch chan struct{}
}

func (w *chanNotifyWriter) Write(p []byte) (int, error) {
	select {
	case w.ch <- struct{}{}:
	default:
	}
	return len(p), nil
}

// listenForPTYOutput returns a tea.Cmd that blocks until the channel receives
// a signal, then returns a PTYOutputMsg.
func listenForPTYOutput(ch <-chan struct{}) tea.Cmd {
	return func() tea.Msg {
		_, ok := <-ch
		if !ok {
			return nil
		}
		return PTYOutputMsg{}
	}
}

// Commands

func (m Model) loadPreviewCmd(s session.Session) tea.Cmd {
	switch m.previewMode {
	case PreviewLive:
		return m.liveCaptureCmd(s)
	case PreviewMessages:
		return func() tea.Msg {
			messages, err := session.ParseJSONL(s.FullPath, m.cfg.MaxJSONLSize, m.cfg.MaxMessages)
			var parsed []ParsedMessage
			for _, msg := range messages {
				parsed = append(parsed, ParsedMessage{Type: msg.Type, Content: msg.Content})
			}
			return PreviewLoadedMsg{SessionID: s.SessionID, Messages: parsed, Err: err}
		}
	case PreviewMeta:
		return func() tea.Msg {
			meta := session.GetMeta(s)
			content := fmt.Sprintf(
				"Session ID:   %s\nProject:      %s\nBranch:       %s\nCreated:      %s\nModified:     %s\nMessages:     %d\n\nFirst prompt:\n%s\n\nSummary:\n%s",
				meta.SessionID, meta.ProjectPath, meta.GitBranch,
				meta.Created, meta.Modified, meta.MsgCount,
				meta.FirstPrompt, meta.Summary,
			)
			return PreviewLoadedMsg{
				SessionID: s.SessionID,
				Messages:  []ParsedMessage{{Type: "summary", Content: content}},
			}
		}
	}
	return nil
}

func (m Model) liveCaptureCmd(s session.Session) tea.Cmd {
	return func() tea.Msg {
		if !m.ptyMgr.IsRunning(s.SessionID) {
			return LiveCaptureMsg{SessionID: s.SessionID, Content: "Session not running"}
		}
		content := m.ptyMgr.Capture(s.SessionID)
		return LiveCaptureMsg{SessionID: s.SessionID, Content: content}
	}
}

func (m Model) embedSessionCmd(s session.Session) tea.Cmd {
	if m.ptyMgr.IsRunning(s.SessionID) {
		return func() tea.Msg {
			return embeddedAttachMsg{sessionID: s.SessionID, projectPath: s.ProjectPath}
		}
	}
	return func() tea.Msg {
		if err := m.ptyMgr.Launch(s.SessionID, s.ProjectPath); err != nil {
			return SessionResumedMsg{Session: s, Err: err}
		}
		return embeddedAttachMsg{sessionID: s.SessionID, projectPath: s.ProjectPath}
	}
}

func (m Model) newSessionCmd() tea.Cmd {
	dir := m.cfg.WorkDir
	if dir == "" {
		dir, _ = os.UserHomeDir()
	}
	return func() tea.Msg {
		sessionID := fmt.Sprintf("new-%d", time.Now().UnixNano())
		if err := m.ptyMgr.LaunchNew(sessionID, dir); err != nil {
			return SessionResumedMsg{Err: err}
		}
		return embeddedAttachMsg{sessionID: sessionID, projectPath: dir}
	}
}

func (m Model) deleteSessionCmd(s session.Session) tea.Cmd {
	return func() tea.Msg {
		err := session.Delete(s)
		return SessionDeletedMsg{SessionID: s.SessionID, Err: err}
	}
}

func (m Model) renameSessionCmd(s session.Session, newSummary string) tea.Cmd {
	return func() tea.Msg {
		err := session.Rename(s, newSummary)
		return SessionRenamedMsg{SessionID: s.SessionID, NewSummary: newSummary, Err: err}
	}
}

func (m Model) exportSessionCmd(s session.Session) tea.Cmd {
	return func() tea.Msg {
		home, _ := os.UserHomeDir()
		exportDir := filepath.Join(home, "claude-exports")
		path, err := session.Export(s, exportDir, m.cfg.MaxJSONLSize, m.cfg.MaxMessages)
		return SessionExportedMsg{SessionID: s.SessionID, Path: path, Err: err}
	}
}

func (m Model) archiveSessionCmd(s session.Session) tea.Cmd {
	return func() tea.Msg {
		err := session.Archive(s)
		return SessionDeletedMsg{SessionID: s.SessionID, Err: err}
	}
}

func (m Model) loadArchivedSessionsCmd() tea.Cmd {
	return func() tea.Msg {
		sessions, projects, err := session.LoadArchived(m.cfg.ClaudeDir)
		return ArchivedSessionsLoadedMsg{
			Sessions: sessions,
			Projects: projects,
			Err:      err,
		}
	}
}

func (m Model) restoreSessionCmd(s session.Session) tea.Cmd {
	return func() tea.Msg {
		err := session.Restore(s)
		return SessionRestoredMsg{SessionID: s.SessionID, Err: err}
	}
}

func (m Model) detectRunningCmd() tea.Cmd {
	return func() tea.Msg {
		running := m.ptyMgr.DetectRunning()
		return RunningSessionsMsg{RunningIDs: running}
	}
}

// enrichRunningCmd reads fresh metadata from JSONL files for running sessions.
func (m Model) enrichRunningCmd() tea.Cmd {
	type info struct {
		id   string
		path string
	}
	var running []info
	for _, s := range m.allSessions {
		if m.runningIDs[s.SessionID] && s.FullPath != "" {
			running = append(running, info{id: s.SessionID, path: s.FullPath})
		}
	}
	if len(running) == 0 {
		return nil
	}
	return func() tea.Msg {
		results := make(enrichedSessionsMsg)
		for _, r := range running {
			firstPrompt, summary, msgCount := session.ExtractMeta(r.path)
			results[r.id] = enrichedMeta{
				firstPrompt: firstPrompt,
				summary:     summary,
				msgCount:    msgCount,
			}
		}
		return results
	}
}

// tickCmd returns a command that sends a tick after the given duration.
type tickMsg time.Time

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(
		d, func(t time.Time) tea.Msg {
			return tickMsg(t)
		},
	)
}
