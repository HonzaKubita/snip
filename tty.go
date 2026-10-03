package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

func isTerminal(f *os.File) bool { return term.IsTerminal(int(f.Fd())) }

// placement remembers where the cursor was before the UI was drawn.
//
// Run from a shell widget, the cursor sits in the middle of the prompt line.
// Drawing there would overwrite the prompt, so the UI goes on the line below
// and the cursor is put back afterwards; that keeps the shell's idea of the
// cursor position correct when it redraws the prompt.
type placement struct {
	tty   *os.File
	moved bool
	col   int // 1-based column to return to, 0 if unknown
}

func enterInline(tty *os.File, captured bool) *placement {
	p := &placement{tty: tty}
	col, ok := cursorColumn(tty)
	if ok {
		p.moved = col > 1
	} else {
		p.moved = captured // stdout captured: most likely a shell widget
	}
	if !p.moved {
		return p
	}
	if ok {
		p.col = col
	} else {
		tty.WriteString("\x1b7") // save cursor, best effort
	}
	tty.WriteString("\r\n")
	return p
}

// leave runs after the UI has cleared itself and left the cursor at the
// start of the first line it used.
func (p *placement) leave() {
	if !p.moved {
		return
	}
	if p.col > 0 {
		fmt.Fprintf(p.tty, "\x1b[1A\x1b[%dG", p.col)
	} else {
		p.tty.WriteString("\x1b8")
	}
}

var dsrRe = regexp.MustCompile(`\x1b\[(\d+);(\d+)R`)

// cursorColumn asks the terminal for the cursor position (DSR).
func cursorColumn(tty *os.File) (int, bool) {
	fd := int(tty.Fd())
	old, err := term.MakeRaw(fd)
	if err != nil {
		return 0, false
	}
	defer term.Restore(fd, old)
	if _, err := tty.WriteString("\x1b[6n"); err != nil {
		return 0, false
	}
	var buf []byte
	chunk := make([]byte, 64)
	deadline := time.Now().Add(300 * time.Millisecond)
	for {
		left := time.Until(deadline)
		if left <= 0 {
			return 0, false
		}
		// select, not poll: poll doesn't work on ttys on macOS.
		var fds unix.FdSet
		fds.Set(fd)
		tv := unix.NsecToTimeval(left.Nanoseconds())
		n, err := unix.Select(fd+1, &fds, nil, nil, &tv)
		if err == unix.EINTR {
			continue
		}
		if err != nil || n == 0 {
			return 0, false
		}
		k, err := unix.Read(fd, chunk)
		if err != nil || k <= 0 {
			return 0, false
		}
		buf = append(buf, chunk[:k]...)
		if m := dsrRe.FindSubmatch(buf); m != nil {
			col, _ := strconv.Atoi(string(m[2]))
			return col, true
		}
	}
}
