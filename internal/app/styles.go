package app

import "github.com/charmbracelet/lipgloss"

// Colors — dark neon palette.
var (
	// Backgrounds (4 depth levels)
	ColorBg        = lipgloss.Color("#0D0D0D")
	ColorBgPanel   = lipgloss.Color("#141419")
	ColorBgActive  = lipgloss.Color("#1A1A2E")
	ColorBgSurface = lipgloss.Color("#1E1E2A")

	// Neon accents
	ColorCyan    = lipgloss.Color("#00E5FF")
	ColorMagenta = lipgloss.Color("#FF2E97")
	ColorGreen   = lipgloss.Color("#39FF14")

	// Text (3 levels)
	ColorTextBright = lipgloss.Color("#E8E8E8")
	ColorTextNormal = lipgloss.Color("#A0A0B0")
	ColorTextMuted  = lipgloss.Color("#5A5A6E")

	// Structural
	ColorBorderDim    = lipgloss.Color("#2A2A3A")
	ColorBorderActive = lipgloss.Color("#00E5FF")

	// Semantic
	ColorWarning = lipgloss.Color("#FF6B35")
	ColorYellow  = lipgloss.Color("#FFD600")
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
	StatusSep     lipgloss.Style
	ListItem      lipgloss.Style
	ListItemSel   lipgloss.Style
	ListItemDim   lipgloss.Style
	Preview       lipgloss.Style
	PreviewTab    lipgloss.Style
	PreviewTabAct lipgloss.Style
	PreviewTitle  lipgloss.Style
	UserMsg       lipgloss.Style
	UserLabel     lipgloss.Style
	AssistantMsg  lipgloss.Style
	AssistantLabel lipgloss.Style
	SummaryMsg    lipgloss.Style
	SummaryLabel  lipgloss.Style
	MsgSeparator  lipgloss.Style
	Running       lipgloss.Style
	RunningDot    lipgloss.Style
	Selected      lipgloss.Style
	SelectedMark  lipgloss.Style
	Age           lipgloss.Style
	MsgCount      lipgloss.Style
	ProjectTag    lipgloss.Style
	HelpKey       lipgloss.Style
	HelpDesc      lipgloss.Style
	Dialog        lipgloss.Style
	DialogTitle   lipgloss.Style
	Error         lipgloss.Style
	ArchiveBadge  lipgloss.Style
	PanelBorder   lipgloss.Style
	PanelTitle    lipgloss.Style
	ConfirmYes    lipgloss.Style
	ConfirmNo     lipgloss.Style
}{
	App: lipgloss.NewStyle().
		Background(ColorBg),

	Title: lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorCyan).
		Padding(0, 1),

	TabActive: lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorCyan).
		Underline(true).
		Padding(0, 1),

	TabInactive: lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Padding(0, 1),

	TabBar: lipgloss.NewStyle(),

	StatusBar: lipgloss.NewStyle().
		Foreground(ColorTextNormal).
		Background(ColorBgPanel).
		Padding(0, 1),

	StatusKey: lipgloss.NewStyle().
		Foreground(ColorTextMuted),

	StatusVal: lipgloss.NewStyle().
		Foreground(ColorTextBright),

	StatusSep: lipgloss.NewStyle().
		Foreground(ColorTextMuted),

	ListItem: lipgloss.NewStyle().
		Padding(0, 1),

	ListItemSel: lipgloss.NewStyle().
		Foreground(ColorCyan).
		Background(ColorBgActive).
		Padding(0, 1).
		Bold(true),

	ListItemDim: lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Padding(0, 1),

	Preview: lipgloss.NewStyle().
		Padding(0, 1),

	PreviewTab: lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Padding(0, 1),

	PreviewTabAct: lipgloss.NewStyle().
		Foreground(ColorCyan).
		Bold(true).
		Padding(0, 1),

	PreviewTitle: lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorCyan),

	UserMsg: lipgloss.NewStyle().
		Foreground(ColorGreen),

	UserLabel: lipgloss.NewStyle().
		Foreground(ColorGreen).
		Bold(true),

	AssistantMsg: lipgloss.NewStyle().
		Foreground(ColorTextBright),

	AssistantLabel: lipgloss.NewStyle().
		Foreground(ColorCyan).
		Bold(true),

	SummaryMsg: lipgloss.NewStyle().
		Foreground(ColorYellow).
		Italic(true),

	SummaryLabel: lipgloss.NewStyle().
		Foreground(ColorYellow).
		Bold(true),

	MsgSeparator: lipgloss.NewStyle().
		Foreground(ColorBorderDim),

	Running: lipgloss.NewStyle().
		Foreground(ColorMagenta).
		Bold(true),

	RunningDot: lipgloss.NewStyle().
		Foreground(ColorMagenta).
		Bold(true),

	Selected: lipgloss.NewStyle().
		Foreground(ColorCyan),

	SelectedMark: lipgloss.NewStyle().
		Foreground(ColorCyan).
		Bold(true),

	Age: lipgloss.NewStyle().
		Foreground(ColorTextMuted),

	MsgCount: lipgloss.NewStyle().
		Foreground(ColorTextNormal),

	ProjectTag: lipgloss.NewStyle().
		Foreground(ColorTextMuted).
		Background(ColorBgPanel).
		Padding(0, 1),

	HelpKey: lipgloss.NewStyle().
		Foreground(ColorCyan).
		Bold(true),

	HelpDesc: lipgloss.NewStyle().
		Foreground(ColorTextMuted),

	Dialog: lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ColorCyan).
		Background(ColorBgSurface).
		Padding(1, 2).
		Width(50),

	DialogTitle: lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorCyan),

	Error: lipgloss.NewStyle().
		Foreground(ColorWarning),

	ArchiveBadge: lipgloss.NewStyle().
		Foreground(lipgloss.Color("#000000")).
		Background(ColorYellow).
		Bold(true).
		Padding(0, 1),

	PanelBorder: lipgloss.NewStyle().
		Foreground(ColorBorderDim),

	PanelTitle: lipgloss.NewStyle().
		Foreground(ColorCyan).
		Bold(true),

	ConfirmYes: lipgloss.NewStyle().
		Foreground(ColorGreen).
		Bold(true),

	ConfirmNo: lipgloss.NewStyle().
		Foreground(ColorMagenta).
		Bold(true),
}
