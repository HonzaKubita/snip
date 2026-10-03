package main

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

type screen int

const (
	screenPick screen = iota
	screenForm
	screenParams
)

// What the session is for.
type purpose int

const (
	pickToUse  purpose = iota // pick, fill in params, hand back the command
	pickToEdit                // pick, then edit
	addNew                    // only the new-snippet form
)

const (
	maxListRows = 10
	maxWrap     = 4 // lines a selected long command may wrap to
)

type app struct {
	st      *styles
	store   *Store
	purpose purpose
	use     string // what enter does with the final command: "run" or "select"

	w, h   int
	screen screen
	status string // one-off message in the footer

	pick   picker
	form   form
	params paramForm

	result   string // the chosen command, params filled in
	message  string // printed once the UI is gone
	canceled bool
	quitting bool
}

type picker struct {
	query   lineInput
	matches []match
	cursor  int
	offset  int
	confirm bool // waiting for y/n to delete the selection
}

type form struct {
	index  int // snippet being edited, -1 for a new one
	fields [3]lineInput
	focus  int
}

type paramForm struct {
	command string
	params  []Param
	inputs  []lineInput
	choice  []int
	focus   int
}

func newApp(store *Store, p purpose, query string) *app {
	a := &app{store: store, purpose: p, use: "select"}
	a.pick.query = lineInput{placeholder: "type to search"}
	a.pick.query.SetValue(query)
	a.refilter(-1)
	return a
}

func (a *app) Init() tea.Cmd { return nil }

func (a *app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.w, a.h = msg.Width, msg.Height
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return a.cancel()
		}
		switch a.screen {
		case screenPick:
			return a.updatePick(msg)
		case screenForm:
			return a.updateForm(msg)
		case screenParams:
			return a.updateParams(msg)
		}
	}
	return a, nil
}

func (a *app) cancel() (tea.Model, tea.Cmd) {
	a.canceled = true
	return a.quit()
}

func (a *app) quit() (tea.Model, tea.Cmd) {
	a.quitting = true
	return a, tea.Quit
}

// width leaves the last column free so lines never trigger auto-wrap.
func (a *app) width() int { return max(a.w-1, 20) }

// View returns nothing once done, which makes Bubble Tea erase the UI.
func (a *app) View() string {
	if a.quitting || a.w == 0 {
		return ""
	}
	var lines []string
	switch a.screen {
	case screenPick:
		lines = a.viewPick()
	case screenForm:
		lines = a.viewForm()
	case screenParams:
		lines = a.viewParams()
	}
	for i, l := range lines {
		lines[i] = truncate(l, a.width())
	}
	return strings.Join(lines, "\n")
}

func (a *app) setStatus(ok bool, msg string) {
	if ok {
		a.status = a.st.ok.Render("✓ " + msg)
	} else {
		a.status = a.st.err.Render("✗ " + msg)
	}
}

// label names a snippet in messages.
func label(sn Snippet) string {
	s := sn.Description
	if s == "" {
		s = strings.ReplaceAll(sn.Command, "\n", " ")
	}
	return "“" + runewidth.Truncate(s, 40, "…") + "”"
}

// ---- picker ----

func (a *app) refilter(keep int) {
	p := &a.pick
	p.matches = filter(a.store.Snippets, p.query.Value())
	p.cursor, p.offset = 0, 0
	for i, m := range p.matches {
		if m.idx == keep {
			p.cursor = i
		}
	}
}

func (a *app) selected() (int, bool) {
	if len(a.pick.matches) == 0 {
		return -1, false
	}
	return a.pick.matches[a.pick.cursor].idx, true
}

func (a *app) move(d int, wrap bool) {
	p := &a.pick
	n := len(p.matches)
	if n == 0 {
		return
	}
	c := p.cursor + d
	if wrap {
		c = (c%n + n) % n
	}
	p.cursor = max(0, min(c, n-1))
}

func (a *app) updatePick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := &a.pick
	if p.confirm {
		p.confirm = false
		if s := msg.String(); s == "y" || s == "Y" {
			a.deleteSelected()
		}
		return a, nil
	}
	a.status = ""
	switch msg.String() {
	case "esc":
		return a.cancel()
	case "enter":
		idx, ok := a.selected()
		if !ok {
			return a, nil
		}
		if a.purpose == pickToEdit {
			a.openForm(idx, a.store.Snippets[idx])
			return a, nil
		}
		return a.choose(idx)
	case "up", "ctrl+p", "ctrl+k":
		a.move(-1, true)
	case "down", "ctrl+j":
		a.move(1, true)
	case "pgup":
		a.move(-a.listHeight(), false)
	case "pgdown":
		a.move(a.listHeight(), false)
	case "ctrl+e":
		if idx, ok := a.selected(); ok {
			a.openForm(idx, a.store.Snippets[idx])
		}
	case "ctrl+n":
		a.openForm(-1, Snippet{})
	case "ctrl+d":
		_, p.confirm = a.selected()
	default:
		if p.query.Update(msg) {
			a.refilter(-1)
		}
	}
	return a, nil
}

func (a *app) choose(idx int) (tea.Model, tea.Cmd) {
	sn := a.store.Snippets[idx]
	params := parseParams(sn.Command)
	if len(params) == 0 {
		a.result = sn.Command
		return a.quit()
	}
	pf := paramForm{
		command: sn.Command,
		params:  params,
		inputs:  make([]lineInput, len(params)),
		choice:  make([]int, len(params)),
	}
	for i, p := range params {
		pf.inputs[i].SetValue(p.Default)
	}
	a.params = pf
	a.screen = screenParams
	return a, nil
}

func (a *app) deleteSelected() {
	idx, ok := a.selected()
	if !ok {
		return
	}
	old := a.store.Snippets
	gone := old[idx]
	a.store.Snippets = append(old[:idx:idx], old[idx+1:]...)
	if err := a.store.Save(); err != nil {
		a.store.Snippets = old
		a.setStatus(false, err.Error())
		return
	}
	cursor := a.pick.cursor
	a.refilter(-1)
	a.pick.cursor = max(0, min(cursor, len(a.pick.matches)-1))
	a.setStatus(true, "deleted "+label(gone))
}

// descWidth is the width of the description column, the same for every
// row so the commands line up.
func (a *app) descWidth(w int) int {
	longest := 0
	for _, sn := range a.store.Snippets {
		longest = max(longest, runewidth.StringWidth(sn.Description))
	}
	if longest == 0 || w < 40 {
		return 0
	}
	return min(longest, max(12, (w-2)/3))
}

type rowLayout struct {
	first, rest int // command width on the first and following lines
	tags        []rune
	chunks      []span
	cut         bool
}

func (a *app) layoutRow(sn Snippet, w, descW int, selected bool) rowLayout {
	cmdW := w - 2
	if descW > 0 {
		cmdW -= descW + 2
	}
	l := rowLayout{first: cmdW, rest: cmdW}
	if t := []rune(tagText(sn.Tags)); len(t) > 0 && cmdW-cellWidth(t)-2 >= 20 {
		l.tags = t
		l.first = cmdW - cellWidth(t) - 2
	}
	lines := 1
	if selected {
		lines = maxWrap
	}
	l.chunks, l.cut = wrapRunes(displayRunes(sn.Command), l.first, l.rest, lines)
	return l
}

// listHeight is fixed for the session (it only changes with the terminal
// size or the number of snippets) so the UI doesn't jump while you type.
func (a *app) listHeight() int {
	w := a.width()
	limit := maxListRows
	if a.h > 0 {
		limit = min(limit, a.h-3)
	}
	descW := a.descWidth(w)
	rows := len(a.store.Snippets)
	for _, sn := range a.store.Snippets {
		rows = max(rows, len(a.layoutRow(sn, w, descW, true).chunks))
	}
	return max(1, min(rows, limit))
}

func (a *app) renderRow(m match, selected bool, w, descW int) []string {
	st := a.st
	sn := a.store.Snippets[m.idx]
	l := a.layoutRow(sn, w, descW, selected)

	bar := "  "
	if selected {
		bar = st.accent.Render("▌") + " "
	}
	first, indent := bar, bar
	if descW > 0 {
		d := displayRunes(sn.Description)
		style := func(c uint8) lipgloss.Style {
			if c == clsMatch {
				return st.match
			}
			if selected {
				return st.bold
			}
			return st.plain
		}
		s, used := clipPaint(d, classes(sn.Description, len(d), false, m.pos[0]), descW, style, st)
		first += s + strings.Repeat(" ", descW-used+2)
		indent += strings.Repeat(" ", descW+2)
	}

	cmd := displayRunes(sn.Command)
	cls := classes(sn.Command, len(cmd), true, m.pos[1])
	style := func(c uint8) lipgloss.Style {
		switch c {
		case clsMatch:
			return st.match
		case clsParam:
			if selected {
				return st.param
			}
			return st.paramFaint
		case clsGlyph:
			return st.faint
		}
		if selected {
			return st.plain
		}
		return st.faint
	}
	var out []string
	for i, ch := range l.chunks {
		s := paint(cmd[ch.start:ch.end], cls[ch.start:ch.end], style)
		if l.cut && i == len(l.chunks)-1 {
			s += st.faint.Render("…")
		}
		if i > 0 {
			out = append(out, indent+s)
			continue
		}
		if len(l.tags) > 0 {
			tagCls := classes("", len(l.tags), false, m.pos[2])
			s = padRight(s, l.first) + "  " + paint(l.tags, tagCls, func(c uint8) lipgloss.Style {
				if c == clsMatch {
					return st.match
				}
				return st.tag
			})
		}
		out = append(out, first+s)
	}
	return out
}

func (a *app) listLines(w, height int) []string {
	st, p := a.st, &a.pick
	var out []string
	switch {
	case len(a.store.Snippets) == 0:
		out = append(out, "  "+st.faint.Render("No snippets yet. Press ")+st.accent.Render("^n")+st.faint.Render(" to add one."))
	case len(p.matches) == 0:
		out = append(out, "  "+st.faint.Render("No matches"))
	default:
		descW := a.descWidth(w)
		sel := a.renderRow(p.matches[p.cursor], true, w, descW)
		if p.cursor < p.offset {
			p.offset = p.cursor
		}
		for p.offset < p.cursor && p.cursor-p.offset+len(sel) > height {
			p.offset++
		}
		for i := p.offset; i < len(p.matches) && len(out) < height; i++ {
			rows := sel
			if i != p.cursor {
				rows = a.renderRow(p.matches[i], false, w, descW)
			}
			out = append(out, rows[:min(len(rows), height-len(out))]...)
		}
	}
	for len(out) < height {
		out = append(out, "")
	}
	return out
}

func (a *app) viewPick() []string {
	st, p, w := a.st, &a.pick, a.width()
	counter := st.faint.Render(fmt.Sprintf("%d/%d", len(p.matches), len(a.store.Snippets)))
	qw := max(w-3-lipgloss.Width(counter), 5)
	lines := []string{st.accentBold.Render("❯ ") + padRight(p.query.View(qw, true, st), qw) + " " + counter}
	lines = append(lines, a.listLines(w, a.listHeight())...)

	var footer string
	switch {
	case p.confirm:
		idx, _ := a.selected()
		footer = st.err.Render("Delete "+label(a.store.Snippets[idx])+"?") + " " + st.bold.Render("y") + st.faint.Render("/n")
	case a.status != "":
		footer = a.status
	case a.purpose == pickToEdit:
		footer = st.help("↵", "edit", "^n", "new", "^d", "delete", "esc", "quit")
	default:
		footer = st.help("↵", a.use, "^e", "edit", "^n", "new", "^d", "delete", "esc", "quit")
	}
	return append(lines, footer)
}

// ---- new / edit form ----

func (a *app) openForm(index int, sn Snippet) {
	f := form{index: index}
	f.fields[0] = lineInput{placeholder: "e.g. ssh <user=root>@<host>", multiline: true, params: true}
	f.fields[1] = lineInput{placeholder: "what it does"}
	f.fields[2] = lineInput{placeholder: "optional, space separated"}
	f.fields[0].SetValue(sn.Command)
	f.fields[1].SetValue(sn.Description)
	f.fields[2].SetValue(strings.Join(sn.Tags, " "))
	if index < 0 && sn.Command != "" {
		f.focus = 1
	}
	a.form = f
	a.status = ""
	a.screen = screenForm
}

func (a *app) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := &a.form
	a.status = ""
	switch msg.String() {
	case "esc":
		if a.purpose == addNew {
			return a.cancel()
		}
		a.screen = screenPick
	case "ctrl+s":
		return a.saveForm()
	case "enter":
		if f.focus == len(f.fields)-1 {
			return a.saveForm()
		}
		f.focus++
	case "tab", "down":
		f.focus = (f.focus + 1) % len(f.fields)
	case "shift+tab", "up":
		f.focus = (f.focus + len(f.fields) - 1) % len(f.fields)
	default:
		f.fields[f.focus].Update(msg)
	}
	return a, nil
}

func (a *app) saveForm() (tea.Model, tea.Cmd) {
	f := &a.form
	sn := Snippet{
		Command:     strings.TrimSpace(f.fields[0].Value()),
		Description: strings.TrimSpace(f.fields[1].Value()),
		Tags:        parseTags(f.fields[2].Value()),
	}
	if sn.Command == "" {
		f.focus = 0
		a.setStatus(false, "the command can't be empty")
		return a, nil
	}
	old := slices.Clone(a.store.Snippets)
	idx, verb := f.index, "updated"
	if idx < 0 {
		a.store.Snippets = append(a.store.Snippets, sn)
		idx, verb = len(a.store.Snippets)-1, "saved"
	} else {
		a.store.Snippets[idx] = sn
	}
	if err := a.store.Save(); err != nil {
		a.store.Snippets = old
		a.setStatus(false, err.Error())
		return a, nil
	}
	if a.purpose != pickToUse {
		a.message = a.st.ok.Render("✓ "+verb) + " " + label(sn)
		return a.quit()
	}
	a.screen = screenPick
	a.refilter(idx)
	if sel, _ := a.selected(); sel != idx {
		a.pick.query.SetValue("") // the query hides it; show everything
		a.refilter(idx)
	}
	a.setStatus(true, verb+" "+label(sn))
	return a, nil
}

func (a *app) viewForm() []string {
	st, f, w := a.st, &a.form, a.width()
	title := "New snippet"
	if f.index >= 0 {
		title = "Edit snippet"
	}
	lines := []string{st.accentBold.Render(title)}
	const labelW = 13
	names := [3]string{"command", "description", "tags"}
	for i := range f.fields {
		marker, name := "  ", st.faint.Render(padRight(names[i], labelW))
		if i == f.focus {
			marker, name = st.accent.Render("❯ "), st.accentBold.Render(padRight(names[i], labelW))
		}
		lines = append(lines, marker+name+f.fields[i].View(w-2-labelW, i == f.focus, st))
	}
	lines = append(lines, strings.Repeat(" ", 2+labelW)+a.paramHint(f.fields[0].Value()))

	back := "back"
	if a.purpose == addNew {
		back = "cancel"
	}
	enter := "next"
	if f.focus == len(f.fields)-1 {
		enter = "save"
	}
	footer := st.help("↵", enter, "⇥", "move", "^s", "save", "esc", back)
	if a.status != "" {
		footer = a.status
	}
	return append(lines, footer)
}

func (a *app) paramHint(cmd string) string {
	st := a.st
	params := parseParams(cmd)
	if len(params) == 0 {
		return st.faint.Render("tip: ") + st.param.Render("<name>") + st.faint.Render(" or ") +
			st.param.Render("<name=default>") + st.faint.Render(" asks for a value when used")
	}
	var parts []string
	for _, p := range params {
		s := st.param.Render(p.Name)
		switch {
		case len(p.Choices) > 0:
			s += st.faint.Render(": " + strings.Join(p.Choices, " | "))
		case p.Default != "":
			s += st.faint.Render(" = " + p.Default)
		}
		parts = append(parts, s)
	}
	return st.faint.Render("asks for ") + strings.Join(parts, st.faint.Render(", "))
}

// ---- parameters ----

func (pf *paramForm) values() []string {
	v := make([]string, len(pf.params))
	for i, p := range pf.params {
		if len(p.Choices) > 0 {
			v[i] = p.Choices[pf.choice[i]]
		} else {
			v[i] = pf.inputs[i].Value()
		}
	}
	return v
}

func (a *app) updateParams(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	pf := &a.params
	n := len(pf.params)
	switch msg.String() {
	case "esc":
		a.screen = screenPick
	case "enter":
		if pf.focus < n-1 {
			pf.focus++
			return a, nil
		}
		a.result, _ = fill(pf.command, pf.params, pf.values(), false)
		return a.quit()
	case "tab", "down":
		pf.focus = (pf.focus + 1) % n
	case "shift+tab", "up":
		pf.focus = (pf.focus + n - 1) % n
	default:
		choices := pf.params[pf.focus].Choices
		if len(choices) == 0 {
			pf.inputs[pf.focus].Update(msg)
			break
		}
		c := &pf.choice[pf.focus]
		switch msg.String() {
		case "left", "ctrl+b":
			*c = (*c + len(choices) - 1) % len(choices)
		case "right", "ctrl+f", " ":
			*c = (*c + 1) % len(choices)
		}
	}
	return a, nil
}

func (a *app) viewParams() []string {
	st, pf, w := a.st, &a.params, a.width()

	// Live preview of the command with the values filled in.
	cmd, spans := fill(pf.command, pf.params, pf.values(), true)
	rs := displayRunes(cmd)
	cls := classes(cmd, len(rs), false, nil)
	for _, sp := range spans {
		c := clsParam
		switch {
		case sp.param == pf.focus:
			c = clsFocus
		case sp.empty:
			c = clsEmpty
		}
		for i := sp.start; i < sp.end; i++ {
			cls[i] = c
		}
	}
	style := func(c uint8) lipgloss.Style {
		switch c {
		case clsParam:
			return st.param
		case clsFocus:
			return st.paramFocus
		case clsEmpty, clsGlyph:
			return st.faint
		}
		return st.plain
	}
	var lines []string
	chunks, cut := wrapRunes(rs, w-2, w-2, maxWrap)
	for i, ch := range chunks {
		prefix := "  "
		if i == 0 {
			prefix = st.accentBold.Render("$ ")
		}
		s := paint(rs[ch.start:ch.end], cls[ch.start:ch.end], style)
		if cut && i == len(chunks)-1 {
			s += st.faint.Render("…")
		}
		lines = append(lines, prefix+s)
	}

	labelW := 0
	for _, p := range pf.params {
		labelW = max(labelW, runewidth.StringWidth(p.Name))
	}
	labelW = min(labelW, 24) + 2
	for i, p := range pf.params {
		focused := i == pf.focus
		name := runewidth.Truncate(p.Name, labelW-2, "…")
		marker, lbl := "  ", st.faint.Render(padRight(name, labelW))
		if focused {
			marker, lbl = st.accent.Render("❯ "), st.accentBold.Render(padRight(name, labelW))
		}
		var field string
		if len(p.Choices) > 0 {
			field = a.choicesView(p.Choices, pf.choice[i], w-2-labelW, focused)
		} else {
			field = pf.inputs[i].View(w-2-labelW, focused, st)
		}
		lines = append(lines, marker+lbl+field)
	}

	enter := "next"
	if pf.focus == len(pf.params)-1 {
		enter = a.use
	}
	keys := []string{"↵", enter, "⇥", "move"}
	if len(pf.params[pf.focus].Choices) > 0 {
		keys = append(keys, "←→", "choose")
	}
	return append(lines, st.help(append(keys, "esc", "back")...))
}

func (a *app) choicesView(opts []string, sel, width int, focused bool) string {
	st := a.st
	var b strings.Builder
	for i, o := range opts {
		switch {
		case i == sel && focused:
			b.WriteString(st.pill.Render(" " + o + " "))
		case i == sel:
			b.WriteString(st.bold.Render(" " + o + " "))
		default:
			b.WriteString(st.faint.Render(" " + o + " "))
		}
	}
	if s := b.String(); lipgloss.Width(s) <= width {
		return s
	}
	return st.faint.Render("‹ ") + st.bold.Render(opts[sel]) + st.faint.Render(fmt.Sprintf(" › %d/%d", sel+1, len(opts)))
}
