package app

import "github.com/charmbracelet/lipgloss"

// Colors used throughout the app.
var (
	ColorPrimary   = lipgloss.Color("#7D56F4")
	ColorSecondary = lipgloss.Color("#6C6C6C")
	ColorAccent    = lipgloss.Color("#04B575")
	ColorWarning   = lipgloss.Color("#FF6347")
	ColorMuted     = lipgloss.Color("#626262")
	ColorBorder    = lipgloss.Color("#383838")
	ColorActiveBg  = lipgloss.Color("#2D2D2D")
	ColorWhite     = lipgloss.Color("#FAFAFA")
	ColorDim       = lipgloss.Color("#4A4A4A")
)

// Styles holds all lip gloss styles.
var Styles = struct {
	App           lipgloss.Style
	Title         lipgloss.Style
	TabActive     lipgloss.Style
	TabInactive   lipgloss.Style
	TabBar        lipgloss.Style
	StatusBar     lipgloss.Style
	StatusKey     lipgloss.Style
	StatusVal     lipgloss.Style
	ListItem      lipgloss.Style
	ListItemSel   lipgloss.Style
	ListItemDim   lipgloss.Style
	Preview       lipgloss.Style
	PreviewTab    lipgloss.Style
	PreviewTabAct lipgloss.Style
	PreviewTitle  lipgloss.Style
	UserMsg       lipgloss.Style
	AssistantMsg  lipgloss.Style
	SummaryMsg    lipgloss.Style
	Running       lipgloss.Style
	Selected      lipgloss.Style
	Age           lipgloss.Style
	MsgCount      lipgloss.Style
	ProjectTag    lipgloss.Style
	HelpKey       lipgloss.Style
	HelpDesc      lipgloss.Style
	Dialog        lipgloss.Style
	DialogTitle   lipgloss.Style
	Error         lipgloss.Style
}{
	App: lipgloss.NewStyle(),

	Title: lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Padding(0, 1),

	TabActive: lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorWhite).
		Background(ColorPrimary).
		Padding(0, 1),

	TabInactive: lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Padding(0, 1),

	TabBar: lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		BorderForeground(ColorBorder),

	StatusBar: lipgloss.NewStyle().
		Foreground(ColorSecondary).
		BorderStyle(lipgloss.NormalBorder()).
		BorderTop(true).
		BorderForeground(ColorBorder).
		Padding(0, 1),

	StatusKey: lipgloss.NewStyle().
		Foreground(ColorMuted),

	StatusVal: lipgloss.NewStyle().
		Foreground(ColorWhite),

	ListItem: lipgloss.NewStyle().
		Padding(0, 1),

	ListItemSel: lipgloss.NewStyle().
		Foreground(ColorWhite).
		Background(ColorActiveBg).
		Padding(0, 1).
		Bold(true),

	ListItemDim: lipgloss.NewStyle().
		Foreground(ColorDim).
		Padding(0, 1),

	Preview: lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderLeft(true).
		BorderForeground(ColorBorder).
		Padding(0, 1),

	PreviewTab: lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Padding(0, 1),

	PreviewTabAct: lipgloss.NewStyle().
		Foreground(ColorWhite).
		Background(ColorPrimary).
		Padding(0, 1),

	PreviewTitle: lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary),

	UserMsg: lipgloss.NewStyle().
		Foreground(ColorAccent),

	AssistantMsg: lipgloss.NewStyle().
		Foreground(ColorWhite),

	SummaryMsg: lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Italic(true),

	Running: lipgloss.NewStyle().
		Foreground(ColorAccent).
		Bold(true),

	Selected: lipgloss.NewStyle().
		Foreground(ColorPrimary),

	Age: lipgloss.NewStyle().
		Foreground(ColorMuted),

	MsgCount: lipgloss.NewStyle().
		Foreground(ColorSecondary),

	ProjectTag: lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Background(lipgloss.Color("#1A1A1A")).
		Padding(0, 1),

	HelpKey: lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true),

	HelpDesc: lipgloss.NewStyle().
		Foreground(ColorSecondary),

	Dialog: lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Padding(1, 2).
		Width(50),

	DialogTitle: lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary),

	Error: lipgloss.NewStyle().
		Foreground(ColorWarning),
}
