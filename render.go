package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

type styles struct {
	plain, bold, faint                lipgloss.Style
	accent, accentBold                lipgloss.Style
	param, paramFaint, paramFocus     lipgloss.Style
	match, tag, ok, err, cursor, pill lipgloss.Style
}

// newStyles uses the 16 ANSI colors so snip follows the terminal's theme.
func newStyles(r *lipgloss.Renderer) *styles {
	magenta, yellow, cyan := lipgloss.Color("5"), lipgloss.Color("3"), lipgloss.Color("6")
	return &styles{
		plain:      r.NewStyle(),
		bold:       r.NewStyle().Bold(true),
		faint:      r.NewStyle().Faint(true),
		accent:     r.NewStyle().Foreground(magenta),
		accentBold: r.NewStyle().Foreground(magenta).Bold(true),
		param:      r.NewStyle().Foreground(yellow),
		paramFaint: r.NewStyle().Foreground(yellow).Faint(true),
		paramFocus: r.NewStyle().Foreground(yellow).Bold(true).Underline(true),
		match:      r.NewStyle().Foreground(magenta).Bold(true),
		tag:        r.NewStyle().Foreground(cyan),
		ok:         r.NewStyle().Foreground(lipgloss.Color("2")),
		err:        r.NewStyle().Foreground(lipgloss.Color("1")),
		cursor:     r.NewStyle().Reverse(true),
		pill:       r.NewStyle().Foreground(magenta).Reverse(true).Bold(true),
	}
}

// Style classes for runs of runes.
const (
	clsBase uint8 = iota
	clsParam
	clsMatch
	clsGlyph
	clsCursor
	clsFocus
	clsEmpty
)

// displayRunes maps text to printable runes one to one, so positions found
// in the original text (matches, params) stay valid.
func displayRunes(s string) []rune {
	rs := []rune(s)
	for i, r := range rs {
		switch {
		case r == '\n':
			rs[i] = '↵'
		case r == '\t':
			rs[i] = ' '
		case r < 0x20 || r == 0x7f:
			rs[i] = '·'
		}
	}
	return rs
}

// classes returns a class per rune: params and newline glyphs of text
// (when wanted) and the given match positions.
func classes(text string, n int, params bool, matches []int) []uint8 {
	cls := make([]uint8, n)
	if params {
		for _, sp := range paramSpans(text) {
			for i := sp.start; i < sp.end && i < n; i++ {
				cls[i] = clsParam
			}
		}
	}
	i := 0
	for _, r := range text {
		if r == '\n' && i < n {
			cls[i] = clsGlyph
		}
		i++
	}
	for _, p := range matches {
		if p < n {
			cls[p] = clsMatch
		}
	}
	return cls
}

func cellWidth(rs []rune) int {
	w := 0
	for _, r := range rs {
		w += runewidth.RuneWidth(r)
	}
	return w
}

// fit returns how many leading runes of rs fit in w cells, and their width.
func fit(rs []rune, w int) (n, used int) {
	for n < len(rs) {
		rw := runewidth.RuneWidth(rs[n])
		if used+rw > w {
			break
		}
		used += rw
		n++
	}
	return n, used
}

// paint renders each run of runes sharing a class with that class's style.
func paint(rs []rune, cls []uint8, style func(uint8) lipgloss.Style) string {
	var b strings.Builder
	for i := 0; i < len(rs); {
		j := i
		for j < len(rs) && cls[j] == cls[i] {
			j++
		}
		b.WriteString(style(cls[i]).Render(string(rs[i:j])))
		i = j
	}
	return b.String()
}

// clipPaint paints as much of rs as fits in w cells, ending in "…" when
// cut, and returns the width used.
func clipPaint(rs []rune, cls []uint8, w int, style func(uint8) lipgloss.Style, st *styles) (string, int) {
	n, used := fit(rs, w)
	if n == len(rs) {
		return paint(rs, cls, style), used
	}
	n, used = fit(rs, w-1)
	return paint(rs[:n], cls[:n], style) + st.faint.Render("…"), used + 1
}

func padRight(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

func truncate(s string, w int) string { return ansi.Truncate(s, w, "…") }

// wrapRunes splits rs into at most maxLines chunks: the first at most first
// cells wide, the rest at most rest cells. Lines break after a space when
// there is one in the second half of the line. If the text doesn't fit, the
// last chunk leaves one cell free for a "…".
func wrapRunes(rs []rune, first, rest, maxLines int) (chunks []span, cut bool) {
	start := 0
	for line := 0; line < maxLines; line++ {
		w := rest
		if line == 0 {
			w = first
		}
		n, _ := fit(rs[start:], w)
		if start+n >= len(rs) {
			return append(chunks, span{start, len(rs)}), false
		}
		if line == maxLines-1 {
			n, _ = fit(rs[start:], w-1)
			return append(chunks, span{start, start + n}), true
		}
		for k := n - 1; k >= n/2 && k > 0; k-- {
			if rs[start+k] == ' ' {
				n = k + 1
				break
			}
		}
		n = max(n, 1)
		chunks = append(chunks, span{start, start + n})
		start += n
	}
	return chunks, false
}

// help renders "key label" pairs for the footer.
func (st *styles) help(pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, st.accent.Render(pairs[i])+" "+st.faint.Render(pairs[i+1]))
	}
	return strings.Join(parts, st.faint.Render("  ·  "))
}
