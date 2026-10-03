package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Snippet is a saved command.
type Snippet struct {
	Description string   `toml:"description,omitempty"`
	Command     string   `toml:"command"`
	Tags        []string `toml:"tags,omitempty"`
}

// Store is the snippet file on disk.
type Store struct {
	Path     string
	Snippets []Snippet
}

type snippetFile struct {
	Snippets []Snippet `toml:"snippet"`
}

const fileHeader = `# snip snippets. Edit by hand or with ` + "`snip edit`" + `.
# Parameters: <name>, <name=default>, <name=|_one_||_two_|>

`

// defaultPath returns $SNIP_FILE, or snippets.toml in the snip config dir.
func defaultPath() (string, error) {
	if p := os.Getenv("SNIP_FILE"); p != "" {
		return p, nil
	}
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "snip", "snippets.toml"), nil
}

func openStore() (*Store, error) {
	path, err := defaultPath()
	if err != nil {
		return nil, err
	}
	return loadStore(path)
}

func loadStore(path string) (*Store, error) {
	s := &Store{Path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var f snippetFile
	if _, err := toml.Decode(string(data), &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	s.Snippets = f.Snippets
	return s, nil
}

// Save writes the snippets atomically. A symlinked file (e.g. from a
// dotfiles repo) is written through, not replaced.
func (s *Store) Save() error {
	var buf bytes.Buffer
	buf.WriteString(fileHeader)
	enc := toml.NewEncoder(&buf)
	enc.Indent = ""
	if err := enc.Encode(snippetFile{s.Snippets}); err != nil {
		return err
	}

	path := s.Path
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".snippets-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// parseTags splits on spaces and commas and drops duplicates.
func parseTags(s string) []string {
	var tags []string
	seen := map[string]bool{}
	for _, t := range strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' }) {
		t = strings.TrimPrefix(t, "#")
		if t != "" && !seen[t] {
			seen[t] = true
			tags = append(tags, t)
		}
	}
	return tags
}

func tagText(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	return "#" + strings.Join(tags, " #")
}
