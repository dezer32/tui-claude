package ptymanager

import tea "github.com/charmbracelet/bubbletea"

// KeyMsgToBytes converts a Bubble Tea key message into raw terminal bytes
// suitable for writing to a PTY.
func KeyMsgToBytes(msg tea.KeyMsg) []byte {
	var raw []byte

	switch msg.Type {
	// Control characters (0x00–0x1f)
	case tea.KeyCtrlA:
		raw = []byte{0x01}
	case tea.KeyCtrlB:
		raw = []byte{0x02}
	case tea.KeyCtrlC:
		raw = []byte{0x03}
	case tea.KeyCtrlD:
		raw = []byte{0x04}
	case tea.KeyCtrlE:
		raw = []byte{0x05}
	case tea.KeyCtrlF:
		raw = []byte{0x06}
	case tea.KeyCtrlG:
		raw = []byte{0x07}
	case tea.KeyCtrlH:
		raw = []byte{0x08} // Backspace on some terminals
	case tea.KeyTab:
		raw = []byte{0x09}
	case tea.KeyCtrlJ:
		raw = []byte{0x0a}
	case tea.KeyCtrlK:
		raw = []byte{0x0b}
	case tea.KeyCtrlL:
		raw = []byte{0x0c}
	case tea.KeyEnter:
		raw = []byte{0x0d}
	case tea.KeyCtrlN:
		raw = []byte{0x0e}
	case tea.KeyCtrlO:
		raw = []byte{0x0f}
	case tea.KeyCtrlP:
		raw = []byte{0x10}
	case tea.KeyCtrlQ:
		raw = []byte{0x11}
	case tea.KeyCtrlR:
		raw = []byte{0x12}
	case tea.KeyCtrlS:
		raw = []byte{0x13}
	case tea.KeyCtrlT:
		raw = []byte{0x14}
	case tea.KeyCtrlU:
		raw = []byte{0x15}
	case tea.KeyCtrlV:
		raw = []byte{0x16}
	case tea.KeyCtrlW:
		raw = []byte{0x17}
	case tea.KeyCtrlX:
		raw = []byte{0x18}
	case tea.KeyCtrlY:
		raw = []byte{0x19}
	case tea.KeyCtrlZ:
		raw = []byte{0x1a}
	case tea.KeyEscape:
		raw = []byte{0x1b}
	case tea.KeyCtrlBackslash:
		raw = []byte{0x1c}
	case tea.KeyCtrlCloseBracket:
		raw = []byte{0x1d}
	case tea.KeyCtrlCaret:
		raw = []byte{0x1e}
	case tea.KeyCtrlUnderscore:
		raw = []byte{0x1f}

	// Special keys
	case tea.KeyBackspace:
		raw = []byte{0x7f}
	case tea.KeySpace:
		raw = []byte{0x20}

	// Arrow keys
	case tea.KeyUp:
		raw = []byte("\x1b[A")
	case tea.KeyDown:
		raw = []byte("\x1b[B")
	case tea.KeyRight:
		raw = []byte("\x1b[C")
	case tea.KeyLeft:
		raw = []byte("\x1b[D")

	// Navigation
	case tea.KeyHome:
		raw = []byte("\x1b[H")
	case tea.KeyEnd:
		raw = []byte("\x1b[F")
	case tea.KeyPgUp:
		raw = []byte("\x1b[5~")
	case tea.KeyPgDown:
		raw = []byte("\x1b[6~")
	case tea.KeyDelete:
		raw = []byte("\x1b[3~")
	case tea.KeyInsert:
		raw = []byte("\x1b[2~")

	// Function keys
	case tea.KeyF1:
		raw = []byte("\x1bOP")
	case tea.KeyF2:
		raw = []byte("\x1bOQ")
	case tea.KeyF3:
		raw = []byte("\x1bOR")
	case tea.KeyF4:
		raw = []byte("\x1bOS")
	case tea.KeyF5:
		raw = []byte("\x1b[15~")
	case tea.KeyF6:
		raw = []byte("\x1b[17~")
	case tea.KeyF7:
		raw = []byte("\x1b[18~")
	case tea.KeyF8:
		raw = []byte("\x1b[19~")
	case tea.KeyF9:
		raw = []byte("\x1b[20~")
	case tea.KeyF10:
		raw = []byte("\x1b[21~")
	case tea.KeyF11:
		raw = []byte("\x1b[23~")
	case tea.KeyF12:
		raw = []byte("\x1b[24~")

	// Shift+Arrow (CSI with modifier 2)
	case tea.KeyShiftUp:
		raw = []byte("\x1b[1;2A")
	case tea.KeyShiftDown:
		raw = []byte("\x1b[1;2B")
	case tea.KeyShiftRight:
		raw = []byte("\x1b[1;2C")
	case tea.KeyShiftLeft:
		raw = []byte("\x1b[1;2D")

	// Ctrl+Arrow (CSI with modifier 5)
	case tea.KeyCtrlUp:
		raw = []byte("\x1b[1;5A")
	case tea.KeyCtrlDown:
		raw = []byte("\x1b[1;5B")
	case tea.KeyCtrlRight:
		raw = []byte("\x1b[1;5C")
	case tea.KeyCtrlLeft:
		raw = []byte("\x1b[1;5D")

	// Shift+Tab
	case tea.KeyShiftTab:
		raw = []byte("\x1b[Z")

	// Runes (printable characters)
	case tea.KeyRunes:
		raw = []byte(string(msg.Runes))

	default:
		// Unknown key type — try runes as fallback
		if len(msg.Runes) > 0 {
			raw = []byte(string(msg.Runes))
		}
	}

	if len(raw) == 0 {
		return nil
	}

	// Alt modifier: prefix with ESC
	if msg.Alt && raw[0] != 0x1b {
		raw = append([]byte{0x1b}, raw...)
	}

	// Bracketed paste
	if msg.Paste {
		raw = append([]byte("\x1b[200~"), raw...)
		raw = append(raw, []byte("\x1b[201~")...)
	}

	return raw
}
