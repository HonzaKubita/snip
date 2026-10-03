package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseParams(t *testing.T) {
	got := parseParams(`ssh <user=root>@<host> -p <port=22> # again: <host> <env=|_dev_||_prod_|>`)
	want := []Param{
		{Name: "user", Default: "root"},
		{Name: "host"},
		{Name: "port", Default: "22"},
		{Name: "env", Default: "dev", Choices: []string{"dev", "prod"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestParseParamsIgnoresShellSyntax(t *testing.T) {
	for _, cmd := range []string{
		`sort < input.txt`,
		`cat <<EOF > out`,
		`diff <(ls a) <(ls b)`,
		`echo "a" 2>&1 >/dev/null`,
		`test 1 < 2 && test 3 > 2`,
	} {
		if p := parseParams(cmd); len(p) != 0 {
			t.Errorf("%q: unexpected params %+v", cmd, p)
		}
	}
}

func TestFill(t *testing.T) {
	cmd := `scp <file> <host>:<file> # ünï <host>`
	params := parseParams(cmd)
	out, spans := fill(cmd, params, []string{"a.txt", ""}, true)
	if want := `scp a.txt <host>:a.txt # ünï <host>`; out != want {
		t.Fatalf("got %q want %q", out, want)
	}
	if len(spans) != 4 || !spans[1].empty || spans[0].empty {
		t.Fatalf("bad spans %+v", spans)
	}
	// rune offsets past multibyte text
	last := spans[3]
	if r := []rune(out); string(r[last.start:last.end]) != "<host>" {
		t.Fatalf("span %+v points at %q", last, string(r[last.start:last.end]))
	}
	out, _ = fill(cmd, params, []string{"a.txt", ""}, false)
	if want := `scp a.txt :a.txt # ünï `; out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestFilter(t *testing.T) {
	snippets := []Snippet{
		{Description: "List docker containers", Command: "docker ps -a", Tags: []string{"docker"}},
		{Description: "Create conda env", Command: "conda create --name <env>"},
		{Description: "Disk usage", Command: "du -sh * | sort -h"},
	}
	if m := filter(snippets, ""); len(m) != 3 || m[0].idx != 0 {
		t.Fatalf("empty query should keep order: %+v", m)
	}
	m := filter(snippets, "cond env")
	if len(m) != 1 || m[0].idx != 1 {
		t.Fatalf("want conda only, got %+v", m)
	}
	// smart case: an uppercase letter makes the term case sensitive
	if m := filter(snippets, "List"); len(m) != 1 {
		t.Fatalf("smart case: got %+v", m)
	}
	if m := filter(snippets, "Docker"); len(m) != 0 {
		t.Fatalf("smart case: got %+v", m)
	}
	if m := filter(snippets, "zzz"); len(m) != 0 {
		t.Fatalf("want no match, got %+v", m)
	}
	// word-start and contiguous matches rank first
	m = filter(snippets, "du")
	if m[0].idx != 2 {
		t.Fatalf("want disk usage first, got %+v", m)
	}
}

func TestWrapRunes(t *testing.T) {
	rs := []rune("abcdefghij")
	chunks, cut := wrapRunes(rs, 4, 3, 4)
	if cut || !reflect.DeepEqual(chunks, []span{{0, 4}, {4, 7}, {7, 10}}) {
		t.Fatalf("got %v %v", chunks, cut)
	}
	chunks, cut = wrapRunes(rs, 4, 3, 2)
	if !cut || !reflect.DeepEqual(chunks, []span{{0, 4}, {4, 6}}) {
		t.Fatalf("got %v %v", chunks, cut)
	}
	if chunks, cut = wrapRunes(nil, 4, 4, 1); cut || len(chunks) != 1 {
		t.Fatalf("empty: got %v %v", chunks, cut)
	}
	// breaks after a space in the second half of the line
	chunks, _ = wrapRunes([]rune("docker run --rm"), 12, 12, 4)
	if !reflect.DeepEqual(chunks, []span{{0, 11}, {11, 15}}) {
		t.Fatalf("word wrap: got %v", chunks)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "snippets.toml")
	s, err := loadStore(path)
	if err != nil || len(s.Snippets) != 0 {
		t.Fatalf("missing file should be empty: %v %v", s, err)
	}
	s.Snippets = []Snippet{
		{Description: "multi", Command: "cat <<EOF\nhi <name>\nEOF", Tags: []string{"a", "b"}},
		{Command: `echo "quotes" 'and' \backslash`},
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	back, err := loadStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back.Snippets, s.Snippets) {
		t.Fatalf("got %+v\nwant %+v", back.Snippets, s.Snippets)
	}
}

func TestSaveThroughSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.toml")
	link := filepath.Join(dir, "link.toml")
	os.WriteFile(real, nil, 0o644)
	os.Symlink(real, link)
	s := &Store{Path: link, Snippets: []Snippet{{Command: "ls"}}}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink was replaced")
	}
	if back, _ := loadStore(real); len(back.Snippets) != 1 {
		t.Fatal("target not written")
	}
}

func TestParseTags(t *testing.T) {
	if got := parseTags(" #git, git  docker,"); !reflect.DeepEqual(got, []string{"git", "docker"}) {
		t.Fatalf("got %v", got)
	}
}
