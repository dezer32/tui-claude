package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	activeTab    int // 0 = All, 1+ = project index
	allMode      bool // true = show all sessions, false = current dir only
	archiveMode  bool // true = viewing archived sessions
	previewMode  PreviewMode

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
}

// NewModel creates a new app model.
func NewModel(cfg config.Config, ptyMgr *ptymanager.Manager) Model {
	// Search input
	si := textinput.New()
	si.Placeholder = "Search sessions..."
	si.Prompt = "/ "

	// Rename input
	ri := textinput.New()
	ri.Placeholder = "New summary..."
	ri.Prompt = "> "

	// List delegate
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(ColorWhite).
		BorderLeftForeground(ColorPrimary)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(ColorSecondary).
		BorderLeftForeground(ColorPrimary)

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
		// Sync VT emulator size with preview viewport for running sessions
		if sel, ok := m.list.SelectedItem().(sessionItem); ok {
			m.ptyMgr.ResizeEmulator(sel.session.SessionID, m.preview.Width, m.preview.Height)
		}
		return m, nil

	case SessionsLoadedMsg:
		if msg.Err != nil {
			m.lastError = msg.Err.Error()
			return m, nil
		}
		m.allSessions = msg.Sessions
		m.projects = msg.Projects
		m.applyFilters()
		return m, nil

	case RunningSessionsMsg:
		m.runningIDs = msg.RunningIDs
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

	case attachMsg:
		return m, tea.Exec(
			&attachExecCmd{mgr: m.ptyMgr, sessionID: msg.session.SessionID},
			func(err error) tea.Msg {
				return SessionResumedMsg{Session: msg.session, Err: err}
			},
		)

	case DialogResultMsg:
		return m.handleDialogResult(msg)

	case SessionDeletedMsg:
		if msg.Err != nil {
			m.lastError = msg.Err.Error()
		} else {
			if m.archiveMode {
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
		return m, nil

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
		}
		if m.previewMode == PreviewLive {
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
		m.archiveMode = !m.archiveMode
		m.activeTab = 0
		if m.archiveMode && len(m.archivedSessions) == 0 {
			m.applyFilters()
			return m, m.loadArchivedSessionsCmd()
		}
		m.applyFilters()
		return m, nil

	case key.Matches(msg, m.keys.Refresh):
		if m.archiveMode {
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
		if m.archiveMode {
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
		if m.archiveMode {
			return m, nil
		}
		if sel, ok := m.list.SelectedItem().(sessionItem); ok {
			return m, m.resumeSessionCmd(sel.session)
		}
		return m, nil

	case key.Matches(msg, m.keys.Export):
		if sel, ok := m.list.SelectedItem().(sessionItem); ok {
			return m, m.exportSessionCmd(sel.session)
		}
		return m, nil

	case key.Matches(msg, m.keys.New):
		if m.archiveMode {
			return m, nil
		}
		return m, m.newSessionCmd()

	case key.Matches(msg, m.keys.Archive):
		if sel, ok := m.list.SelectedItem().(sessionItem); ok {
			if m.archiveMode {
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

	return lipgloss.JoinVertical(lipgloss.Left,
		tabBar,
		content,
		statusBar,
	)
}

func (m *Model) updateLayout() {
	tabBarHeight := 1
	statusBarHeight := 1
	contentHeight := m.height - tabBarHeight - statusBarHeight - 2 // borders

	listWidth := m.width * 2 / 5
	if !m.cfg.PreviewEnabled {
		listWidth = m.width
	}

	m.list.SetSize(listWidth, contentHeight)
	m.preview.Width = m.width - listWidth - 3 // border + padding
	m.preview.Height = contentHeight
}

func (m *Model) applyFilters() {
	var filtered []session.Session
	var activeProjects []session.Project

	if m.archiveMode {
		filtered = m.archivedSessions
		activeProjects = m.archivedProjects
	} else {
		filtered = m.allSessions
		activeProjects = m.projects
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
		if !m.archiveMode {
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
		prefix = "● "
	}
	if i.session.Selected {
		prefix = "✓ "
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
	title := Styles.Title.Render("tui-claude")

	if m.archiveMode {
		badge := Styles.ArchiveBadge.Render("ARCHIVED")
		bar := lipgloss.JoinHorizontal(lipgloss.Center, title, "  ", badge)

		if m.allMode {
			tabs := []string{"All"}
			for _, p := range m.archivedProjects {
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
			bar = lipgloss.JoinHorizontal(lipgloss.Center, title, "  ", badge, "  ", tabLine)
		}

		return Styles.TabBar.Width(m.width).Render(bar)
	}

	if !m.allMode {
		// Current directory mode: show project name
		dirName := filepath.Base(m.cfg.WorkDir)
		dirLabel := Styles.TabActive.Render("@ " + dirName)
		bar := lipgloss.JoinHorizontal(lipgloss.Center, title, "  ", dirLabel)
		return Styles.TabBar.Width(m.width).Render(bar)
	}

	// All mode: show project tabs
	tabs := []string{"All"}
	for _, p := range m.projects {
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
	bar := lipgloss.JoinHorizontal(lipgloss.Center, title, "  ", tabLine)
	return Styles.TabBar.Width(m.width).Render(bar)
}

func (m Model) renderContent() string {
	listView := m.list.View()

	// Show hint when no sessions
	if len(m.sessions) == 0 {
		var hint string
		if m.archiveMode {
			hint = Styles.HelpDesc.Render("No archived sessions.\nPress " + Styles.HelpKey.Render("V") + " to go back.")
		} else if !m.allMode && len(m.allSessions) > 0 {
			hint = Styles.HelpDesc.Render("No sessions for current directory.\nPress " + Styles.HelpKey.Render("a") + " to show all projects.")
		}
		if hint != "" {
			listView = lipgloss.Place(m.width*2/5, m.height-4, lipgloss.Center, lipgloss.Center, hint)
		}
	}

	if !m.cfg.PreviewEnabled {
		return listView
	}

	// Preview panel
	previewTabs := m.renderPreviewTabs()
	previewContent := m.preview.View()
	previewPanel := lipgloss.JoinVertical(lipgloss.Left, previewTabs, previewContent)
	previewPanel = Styles.Preview.Height(m.height - 4).Render(previewPanel)

	return lipgloss.JoinHorizontal(lipgloss.Top, listView, previewPanel)
}

func (m Model) renderPreviewTabs() string {
	modes := []PreviewMode{PreviewLive, PreviewMessages, PreviewMeta}
	var tabs []string
	for _, mode := range modes {
		if mode == m.previewMode {
			tabs = append(tabs, Styles.PreviewTabAct.Render("["+mode.String()+"]"))
		} else {
			tabs = append(tabs, Styles.PreviewTab.Render("["+mode.String()+"]"))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, tabs...)
}

func (m Model) renderStatusBar() string {
	sessionCount := itoa(len(m.sessions))
	sort := m.sortField.String()

	var projectCount string
	if m.archiveMode {
		projectCount = itoa(len(m.archivedProjects))
	} else {
		projectCount = itoa(len(m.projects))
	}

	mode := "Current dir"
	if m.allMode {
		mode = "All projects"
	}

	left := sessionCount + " sessions | " + projectCount + " projects | Sort: " + sort + " | " + mode
	right := "q:quit /:search a:toggle V:archive ?:help"

	if m.archiveMode {
		left = "[ARCHIVED] " + left
		right = "A:restore d:delete e:export V:back ?:help"
	}

	if m.lastError != "" {
		left = Styles.Error.Render("Error: " + m.lastError)
	}

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 0 {
		gap = 0
	}
	padding := ""
	for i := 0; i < gap; i++ {
		padding += " "
	}

	return Styles.StatusBar.Width(m.width).Render(left + padding + right)
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
		"Press " + Styles.HelpKey.Render("y") + " to confirm, " +
		Styles.HelpKey.Render("n") + " to cancel"
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
		BorderForeground(ColorPrimary).
		Padding(0, 1).
		Width(40).
		Render(m.searchInput.View())
	return placeOverlay(m.width, m.height-2, searchBox, base)
}

func (m Model) renderHelp() string {
	var bindings []struct{ key, desc string }

	if m.archiveMode {
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
			{"V", "Back to active sessions"},
			{"?", "This help"},
			{"R", "Refresh list"},
			{"q", "Quit"},
		}
	} else {
		bindings = []struct{ key, desc string }{
			{"↑/k, ↓/j", "Navigate list"},
			{"Enter", "Resume session"},
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
			{"V", "View archived sessions"},
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

	var out string
	for _, msg := range msgs {
		switch msg.Type {
		case "user":
			out += Styles.UserMsg.Render("> "+msg.Content) + "\n\n"
		case "assistant":
			if renderer != nil {
				if rendered, err := renderer.Render(msg.Content); err == nil {
					out += rendered + "\n"
				} else {
					out += Styles.AssistantMsg.Render(msg.Content) + "\n\n"
				}
			} else {
				out += Styles.AssistantMsg.Render(msg.Content) + "\n\n"
			}
		case "summary":
			out += Styles.SummaryMsg.Render("Summary: "+msg.Content) + "\n\n"
		}
	}
	return out
}

// placeOverlay places a dialog box centered over the base content.
func placeOverlay(width, height int, overlay, base string) string {
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center,
		overlay,
		lipgloss.WithWhitespaceChars(" "),
	)
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

func (m Model) resumeSessionCmd(s session.Session) tea.Cmd {
	if m.ptyMgr.IsRunning(s.SessionID) {
		// Already running — attach to it
		return tea.Exec(
			&attachExecCmd{mgr: m.ptyMgr, sessionID: s.SessionID},
			func(err error) tea.Msg {
				return SessionResumedMsg{Session: s, Err: err}
			},
		)
	}
	// Launch in PTY, then attach
	return func() tea.Msg {
		if err := m.ptyMgr.Launch(s.SessionID, s.ProjectPath); err != nil {
			return SessionResumedMsg{Session: s, Err: err}
		}
		return attachMsg{session: s}
	}
}

func (m Model) newSessionCmd() tea.Cmd {
	return func() tea.Msg {
		home, _ := os.UserHomeDir()
		sessionID := fmt.Sprintf("new-%d", time.Now().UnixNano())
		if err := m.ptyMgr.LaunchNew(sessionID, home); err != nil {
			return SessionResumedMsg{Err: err}
		}
		return attachMsg{session: session.Session{SessionID: sessionID, ProjectPath: home}}
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

// attachMsg signals that a PTY session was launched and needs attaching.
type attachMsg struct {
	session session.Session
}

// attachExecCmd implements tea.ExecCommand for in-process PTY attach.
type attachExecCmd struct {
	mgr       *ptymanager.Manager
	sessionID string
}

func (a *attachExecCmd) Run() error {
	return ptymanager.AttachFunc(a.mgr, a.sessionID)()
}

func (a *attachExecCmd) SetStdin(io.Reader)  {}
func (a *attachExecCmd) SetStdout(io.Writer) {}
func (a *attachExecCmd) SetStderr(io.Writer) {}

// tickCmd returns a command that sends a tick after the given duration.
type tickMsg time.Time

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
