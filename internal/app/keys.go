package app

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines all keybindings.
type KeyMap struct {
	Up        key.Binding
	Down      key.Binding
	Enter     key.Binding
	New       key.Binding
	Delete    key.Binding
	Rename    key.Binding
	Search    key.Binding
	Sort      key.Binding
	Group     key.Binding
	TabSwitch key.Binding
	Space     key.Binding
	Export    key.Binding
	Archive   key.Binding
	ToggleAll key.Binding
	Help      key.Binding
	Stats     key.Binding
	Refresh   key.Binding
	Quit      key.Binding
	TabLeft   key.Binding
	TabRight  key.Binding
	Escape    key.Binding
}

// DefaultKeyMap returns the default keybindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "resume session"),
		),
		New: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "new session"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "delete"),
		),
		Rename: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "rename"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		Sort: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "cycle sort"),
		),
		Group: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "group by project"),
		),
		TabSwitch: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch preview"),
		),
		Space: key.NewBinding(
			key.WithKeys(" "),
			key.WithHelp("space", "multi-select"),
		),
		Export: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "export"),
		),
		Archive: key.NewBinding(
			key.WithKeys("A"),
			key.WithHelp("A", "archive"),
		),
		ToggleAll: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "toggle all/current dir"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Stats: key.NewBinding(
			key.WithKeys("S"),
			key.WithHelp("S", "statistics"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("R"),
			key.WithHelp("R", "refresh"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		TabLeft: key.NewBinding(
			key.WithKeys("h"),
			key.WithHelp("h", "prev tab"),
		),
		TabRight: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "next tab"),
		),
		Escape: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
	}
}
