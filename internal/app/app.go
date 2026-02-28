package app

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vladislav-k/tui-claude/internal/config"
	"github.com/vladislav-k/tui-claude/internal/session"
	"github.com/vladislav-k/tui-claude/internal/tmux"
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
	cfg     config.Config
	keys    KeyMap
	state   State
	width   int
	height  int
	hasTmux bool

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
	previewMode  PreviewMode

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
func NewModel(cfg config.Config) Model {
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
		hasTmux:     config.HasTmux(),
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
	cmds := []tea.Cmd{
		loadSessionsCmd(m.cfg.ClaudeDir),
		tickCmd(2 * time.Second),
	}
	if m.hasTmux {
		cmds = append(cmds, detectRunningCmd())
	}
	return tea.Batch(cmds...)
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
		// Match short IDs to full session IDs
		var sessionIDs []string
		for _, s := range m.allSessions {
			sessionIDs = append(sessionIDs, s.SessionID)
		}
		m.runningIDs = tmux.MatchRunning(msg.RunningIDs, sessionIDs)
		m.applyFilters()
		return m, nil

	case PreviewLoadedMsg:
		if msg.Err != nil {
			m.previewContent = "Could not parse session data"
		} else {
			m.previewSession = msg.SessionID
			m.previewContent = renderMessages(msg.Messages)
		}
		m.preview.SetContent(m.previewContent)
		m.preview.GotoTop()
		return m, nil

	case LiveCaptureMsg:
		if msg.Err == nil {
			m.previewContent = msg.Content
			m.preview.SetContent(m.previewContent)
		}
		return m, nil

	case DialogResultMsg:
		return m.handleDialogResult(msg)

	case SessionDeletedMsg:
		if msg.Err != nil {
			m.lastError = msg.Err.Error()
		} else {
			m.removeSession(msg.SessionID)
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
		var cmds []tea.Cmd
		if m.hasTmux {
			cmds = append(cmds, detectRunningCmd())
		}
		if m.previewMode == PreviewLive {
			if sel, ok := m.list.SelectedItem().(sessionItem); ok {
				cmds = append(cmds, m.liveCaptureCmd(sel.session))
			}
		}
		cmds = append(cmds, tickCmd(2*time.Second))
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

	case key.Matches(msg, m.keys.Refresh):
		return m, loadSessionsCmd(m.cfg.ClaudeDir)

	case key.Matches(msg, m.keys.TabSwitch):
		m.previewMode = m.previewMode.Next()
		if sel, ok := m.list.SelectedItem().(sessionItem); ok {
			return m, m.loadPreviewCmd(sel.session)
		}
		return m, nil

	case key.Matches(msg, m.keys.TabLeft):
		if m.activeTab > 0 {
			m.activeTab--
			m.applyFilters()
		}
		return m, nil

	case key.Matches(msg, m.keys.TabRight):
		if m.activeTab < len(m.projects) {
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
		if sel, ok := m.list.SelectedItem().(sessionItem); ok {
			m.state = StateRenameDialog
			m.renameInput.Reset()
			m.renameInput.SetValue(sel.session.Summary)
			m.renameInput.Focus()
			return m, textinput.Blink
		}
		return m, nil

	case key.Matches(msg, m.keys.Enter):
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
		return m, m.newSessionCmd()

	case key.Matches(msg, m.keys.Archive):
		if sel, ok := m.list.SelectedItem().(sessionItem); ok {
			m.state = StateConfirmDialog
			m.confirmAction = ConfirmArchive
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
	filtered := m.allSessions

	// Tab filter
	if m.activeTab > 0 && m.activeTab <= len(m.projects) {
		filtered = session.FilterByProject(filtered, m.projects[m.activeTab-1].Path)
	}

	// Sort
	session.SortSessions(filtered, m.sortField)

	m.sessions = filtered

	// Update list items
	items := make([]list.Item, len(filtered))
	for i, s := range filtered {
		s.IsRunning = m.runningIDs[s.SessionID]
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

	title := Styles.Title.Render("tui-claude")
	tabLine := lipgloss.JoinHorizontal(lipgloss.Center, rendered...)

	bar := lipgloss.JoinHorizontal(lipgloss.Center, title, "  ", tabLine)
	return Styles.TabBar.Width(m.width).Render(bar)
}

func (m Model) renderContent() string {
	listView := m.list.View()

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
	projectCount := itoa(len(m.projects))
	sort := m.sortField.String()

	left := sessionCount + " sessions | " + projectCount + " projects | Sort: " + sort
	right := "q:quit /:search ?:help"

	if m.lastError != "" {
		left = Styles.Error.Render("Error: " + m.lastError)
	}

	if !m.hasTmux {
		right = "[view-only] " + right
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
	if m.confirmAction == ConfirmArchive {
		title = "Archive session?"
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
	bindings := []struct{ key, desc string }{
		{"↑/k, ↓/j", "Navigate list"},
		{"Enter", "Resume session in tmux"},
		{"n", "New Claude session"},
		{"d", "Delete session"},
		{"r", "Rename (edit summary)"},
		{"/", "Search sessions"},
		{"s", "Cycle sort (date/project/messages)"},
		{"Tab", "Switch preview mode"},
		{"Space", "Multi-select"},
		{"e", "Export to markdown"},
		{"h/l", "Switch project tabs"},
		{"?", "This help"},
		{"S", "Statistics"},
		{"R", "Refresh list"},
		{"q", "Quit"},
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

func renderMessages(msgs []ParsedMessage) string {
	var out string
	for _, msg := range msgs {
		switch msg.Type {
		case "user":
			out += Styles.UserMsg.Render("> "+msg.Content) + "\n\n"
		case "assistant":
			out += Styles.AssistantMsg.Render(msg.Content) + "\n\n"
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
		windowName := tmux.WindowName(s.ShortID())
		if !tmux.WindowExists(windowName) {
			return LiveCaptureMsg{SessionID: s.SessionID, Content: "Session not running in tmux"}
		}
		content, err := tmux.CapturPane(windowName)
		return LiveCaptureMsg{SessionID: s.SessionID, Content: content, Err: err}
	}
}

func (m Model) resumeSessionCmd(s session.Session) tea.Cmd {
	return func() tea.Msg {
		if !m.hasTmux {
			return SessionResumedMsg{Session: s, Err: fmt.Errorf("tmux not available")}
		}
		err := tmux.CreateWindow(s.SessionID, s.ShortID(), s.ProjectPath)
		return SessionResumedMsg{Session: s, Err: err}
	}
}

func (m Model) newSessionCmd() tea.Cmd {
	return func() tea.Msg {
		if !m.hasTmux {
			return SessionResumedMsg{Err: fmt.Errorf("tmux not available")}
		}
		home, _ := os.UserHomeDir()
		err := tmux.CreateNewSession(home)
		return SessionResumedMsg{Err: err}
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

func detectRunningCmd() tea.Cmd {
	return func() tea.Msg {
		shortIDs := tmux.DetectRunning()
		return RunningSessionsMsg{RunningIDs: shortIDs}
	}
}

// tickCmd returns a command that sends a tick after the given duration.
type tickMsg time.Time

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
