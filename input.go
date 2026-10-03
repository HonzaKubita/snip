package main

import (
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// lineInput is a one-line editor with emacs-style keys and horizontal
// scrolling. It can highlight <param> placeholders while you type.
type lineInput struct {
	runes       []rune
	pos         int // cursor, in runes
	off         int // first visible rune
	placeholder string
	multiline   bool // keep newlines from pastes (shown as ↵)
	params      bool // highlight <param> placeholders
}

func (in *lineInput) SetValue(s string) {
	in.runes = []rune(s)
	in.pos = len(in.runes)
	in.off = 0
}

func (in *lineInput) Value() string { return string(in.runes) }

// Update applies an editing key and reports whether it was one.
func (in *lineInput) Update(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyRunes:
		if !msg.Alt {
			in.insert(msg.Runes)
			break
		}
		switch string(msg.Runes) {
		case "b":
			in.pos = in.wordLeft()
		case "f":
			in.pos = in.wordRight()
		case "d":
			in.delete(in.pos, in.wordRight())
		default:
			return false
		}
	case tea.KeySpace:
		in.insert([]rune{' '})
	case tea.KeyBackspace, tea.KeyCtrlH:
		if msg.Alt {
			in.delete(in.wordLeft(), in.pos)
		} else if in.pos > 0 {
			in.delete(in.pos-1, in.pos)
		}
	case tea.KeyDelete, tea.KeyCtrlD:
		if in.pos < len(in.runes) {
			in.delete(in.pos, in.pos+1)
		}
	case tea.KeyCtrlW:
		in.delete(in.wordLeft(), in.pos)
	case tea.KeyCtrlU:
		in.delete(0, in.pos)
	case tea.KeyCtrlK:
		in.delete(in.pos, len(in.runes))
	case tea.KeyLeft, tea.KeyCtrlB:
		if msg.Alt {
			in.pos = in.wordLeft()
		} else if in.pos > 0 {
			in.pos--
		}
	case tea.KeyRight, tea.KeyCtrlF:
		if msg.Alt {
			in.pos = in.wordRight()
		} else if in.pos < len(in.runes) {
			in.pos++
		}
	case tea.KeyCtrlLeft:
		in.pos = in.wordLeft()
	case tea.KeyCtrlRight:
		in.pos = in.wordRight()
	case tea.KeyHome, tea.KeyCtrlA:
		in.pos = 0
	case tea.KeyEnd, tea.KeyCtrlE:
		in.pos = len(in.runes)
	default:
		return false
	}
	return true
}

func (in *lineInput) insert(rs []rune) {
	s := strings.NewReplacer("\r\n", "\n", "\r", "\n", "\t", " ").Replace(string(rs))
	s = strings.TrimRight(s, "\n")
	if !in.multiline {
		s = strings.ReplaceAll(s, "\n", " ")
	}
	clean := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '\n' || !unicode.IsControl(r) {
			clean = append(clean, r)
		}
	}
	rest := append(clean, in.runes[in.pos:]...)
	in.runes = append(in.runes[:in.pos], rest...)
	in.pos += len(clean)
}

func (in *lineInput) delete(from, to int) {
	if from >= to {
		return
	}
	in.runes = append(in.runes[:from], in.runes[to:]...)
	in.pos = from
}

func isWord(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

func (in *lineInput) wordLeft() int {
	i := in.pos
	for i > 0 && !isWord(in.runes[i-1]) {
		i--
	}
	for i > 0 && isWord(in.runes[i-1]) {
		i--
	}
	return i
}

func (in *lineInput) wordRight() int {
	i := in.pos
	for i < len(in.runes) && !isWord(in.runes[i]) {
		i++
	}
	for i < len(in.runes) && isWord(in.runes[i]) {
		i++
	}
	return i
}

// View renders the input in exactly width cells or fewer. A focused input
// scrolls to keep the cursor visible; an unfocused one shows its start.
func (in *lineInput) View(width int, focused bool, st *styles) string {
	if width < 2 {
		return ""
	}
	if len(in.runes) == 0 {
		ph := []rune(in.placeholder)
		if !focused {
			return st.faint.Render(truncate(in.placeholder, width))
		}
		if len(ph) == 0 {
			return st.cursor.Render(" ")
		}
		return st.cursor.Faint(true).Render(string(ph[0])) + st.faint.Render(truncate(string(ph[1:]), width-1))
	}

	disp := displayRunes(string(in.runes))
	cls := classes(string(in.runes), len(disp), in.params, nil)
	style := func(c uint8) lipgloss.Style {
		switch c {
		case clsParam:
			return st.param
		case clsGlyph:
			return st.faint
		case clsCursor:
			return st.cursor
		}
		return st.plain
	}

	if !focused {
		s, _ := clipPaint(disp, cls, width, style, st)
		return s
	}

	// Scroll so the cursor (one cell, even at the end) stays visible.
	if in.pos < in.off {
		in.off = in.pos
	}
	for in.off < in.pos && cellWidth(disp[in.off:in.pos])+1 > width {
		in.off++
	}
	for in.off > 0 && cellWidth(disp[in.off-1:])+1 <= width {
		in.off--
	}
	if in.pos < len(cls) {
		cls[in.pos] = clsCursor
	}
	n, _ := fit(disp[in.off:], width)
	s := paint(disp[in.off:in.off+n], cls[in.off:in.off+n], style)
	if in.pos == len(disp) {
		s += st.cursor.Render(" ")
	}
	return s
}
