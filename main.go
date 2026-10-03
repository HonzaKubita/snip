// snip is a small command snippet manager with an inline terminal UI.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var version = "0.1.0"

const usage = `snip - save shell commands and get them back fast

Usage:
  snip [query]           pick a snippet and run it
  snip search [-q query] pick a snippet and print it (used by the shell widget)
  snip exec [-q query]   pick a snippet and run it
  snip new [command]     save a new snippet (the command is prefilled if given)
  snip edit [-q query]   pick a snippet and edit it
  snip edit --file       open the snippet file in $EDITOR
  snip list              print all snippets
  snip init zsh          print the zsh integration (Ctrl-S widget)
  snip path              print where snippets are stored

Parameters in commands:
  <name>                 ask for a value
  <name=default>         ask, with a default
  <name=|_one_||_two_|>  pick one of the options

Picker keys:
  type to filter · ↑/↓ move · enter use · ^e edit · ^n new · ^d delete · esc quit

Snippets live in $SNIP_FILE or ~/.config/snip/snippets.toml.
`

var errCanceled = errors.New("canceled")

type exitCode int

func (e exitCode) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

func main() {
	err := run(os.Args[1:])
	var code exitCode
	switch {
	case err == nil:
	case errors.Is(err, errCanceled):
		os.Exit(130)
	case errors.As(err, &code):
		os.Exit(int(code))
	case errors.Is(err, flag.ErrHelp):
	default:
		fmt.Fprintln(os.Stderr, "snip:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "search", "select":
		return cmdUse(args, false)
	case "exec", "run":
		return cmdUse(args, true)
	case "new", "add":
		return cmdNew(args)
	case "edit":
		return cmdEdit(args)
	case "list", "ls":
		return cmdList()
	case "init":
		return cmdInit(args)
	case "path":
		path, err := defaultPath()
		if err == nil {
			fmt.Println(path)
		}
		return err
	case "help":
		fmt.Print(usage)
		return nil
	case "version":
		fmt.Println("snip", version)
		return nil
	case "":
		for _, a := range args {
			if a == "-v" || a == "--version" {
				fmt.Println("snip", version)
				return nil
			}
		}
		return cmdUse(args, isTerminal(os.Stdout))
	default:
		// Anything else is a search: `snip docker` opens the picker filtered.
		return cmdUse(append([]string{cmd}, args...), isTerminal(os.Stdout))
	}
}

func flags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	return fs
}

func parse(fs *flag.FlagSet, args []string) error {
	err := fs.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Print(usage)
	}
	return err
}

func cmdUse(args []string, execute bool) error {
	fs := flags("search")
	var query string
	fs.StringVar(&query, "q", "", "")
	fs.StringVar(&query, "query", "", "")
	if err := parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		query = strings.TrimSpace(query + " " + strings.Join(fs.Args(), " "))
	}
	store, err := openStore()
	if err != nil {
		return err
	}
	a := newApp(store, pickToUse, query)
	if execute {
		a.use = "run"
	}
	if err := runUI(a); err != nil {
		return err
	}
	if execute {
		return runCommand(a.result)
	}
	fmt.Println(a.result)
	return nil
}

func cmdNew(args []string) error {
	fs := flags("new")
	var desc, tags string
	fs.StringVar(&desc, "d", "", "")
	fs.StringVar(&desc, "description", "", "")
	fs.StringVar(&tags, "t", "", "")
	fs.StringVar(&tags, "tags", "", "")
	if err := parse(fs, args); err != nil {
		return err
	}
	store, err := openStore()
	if err != nil {
		return err
	}
	a := newApp(store, addNew, "")
	a.openForm(-1, Snippet{Command: strings.Join(fs.Args(), " "), Description: desc, Tags: parseTags(tags)})
	return runUI(a)
}

func cmdEdit(args []string) error {
	fs := flags("edit")
	var query string
	var file bool
	fs.StringVar(&query, "q", "", "")
	fs.StringVar(&query, "query", "", "")
	fs.BoolVar(&file, "f", false, "")
	fs.BoolVar(&file, "file", false, "")
	if err := parse(fs, args); err != nil {
		return err
	}
	store, err := openStore()
	if err != nil {
		if file {
			path, _ := defaultPath()
			return openEditor(path) // let them fix a broken file
		}
		return err
	}
	if file {
		if _, err := os.Stat(store.Path); errors.Is(err, os.ErrNotExist) {
			if err := store.Save(); err != nil {
				return err
			}
		}
		return openEditor(store.Path)
	}
	if fs.NArg() > 0 {
		query = strings.TrimSpace(query + " " + strings.Join(fs.Args(), " "))
	}
	return runUI(newApp(store, pickToEdit, query))
}

func cmdList() error {
	store, err := openStore()
	if err != nil {
		return err
	}
	st := newStyles(lipgloss.DefaultRenderer())
	for i, sn := range store.Snippets {
		if i > 0 {
			fmt.Println()
		}
		head := st.bold.Render(sn.Description)
		if sn.Description == "" {
			head = st.faint.Render("(no description)")
		}
		if len(sn.Tags) > 0 {
			head += "  " + st.tag.Render(tagText(sn.Tags))
		}
		fmt.Println(head)
		var out []string
		for _, line := range strings.Split(sn.Command, "\n") {
			rs := []rune(line)
			out = append(out, "  "+paint(rs, classes(line, len(rs), true, nil), func(c uint8) lipgloss.Style {
				if c == clsParam {
					return st.param
				}
				return st.plain
			}))
		}
		fmt.Println(strings.Join(out, "\n"))
	}
	return nil
}

func cmdInit(args []string) error {
	shell := filepath.Base(os.Getenv("SHELL"))
	if len(args) > 0 {
		shell = args[0]
	}
	script, ok := shellScripts[shell]
	if !ok {
		return fmt.Errorf("no %s integration yet, only zsh (`snip search` prints the picked command if you want to wire one up)", shell)
	}
	fmt.Print(script)
	return nil
}

// runUI shows the app inline on the terminal, even when stdout is captured
// (as in the shell widget), and erases it again when done.
func runUI(a *app) error {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return errors.New("snip needs an interactive terminal")
	}
	defer tty.Close()
	a.st = newStyles(lipgloss.NewRenderer(tty))

	place := enterInline(tty, !isTerminal(os.Stdout))
	_, err = tea.NewProgram(a, tea.WithInput(tty), tea.WithOutput(tty)).Run()
	place.leave()
	if err != nil {
		return err
	}
	if a.canceled {
		return errCanceled
	}
	if a.message != "" {
		fmt.Fprintln(os.Stderr, a.message)
	}
	return nil
}

func runCommand(command string) error {
	st := newStyles(lipgloss.NewRenderer(os.Stderr))
	fmt.Fprintln(os.Stderr, st.faint.Render("$ "+command))

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	c := exec.Command(shell, "-c", command)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr

	// Ctrl-C goes to the command; snip just waits for it to finish.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)

	err := c.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitCode(max(exitErr.ExitCode(), 1))
	}
	return err
}

func openEditor(path string) error {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	// Through sh so EDITOR can carry arguments, like "code --wait".
	c := exec.Command("sh", "-c", editor+` "$1"`, "sh", path)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return err
	}
	if _, err := loadStore(path); err != nil {
		return fmt.Errorf("the file has an error now: %w", err)
	}
	return nil
}
