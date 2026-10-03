package main

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// A parameter is <name>, <name=default> or <name=|_one_||_two_|> (choices,
// same syntax as pet). Names start with a letter or underscore, so shell
// syntax like `< file`, `<<EOF` or `<(cmd)` is left alone.
var (
	paramRe   = regexp.MustCompile(`<([A-Za-z_][A-Za-z0-9_.\-]*)(?:=([^<>]*))?>`)
	choicesRe = regexp.MustCompile(`^(?:\|_.*?_\|)+$`)
	choiceRe  = regexp.MustCompile(`\|_(.*?)_\|`)
)

type Param struct {
	Name    string
	Default string
	Choices []string
}

// parseParams returns the parameters of cmd in order of first appearance.
// A name used several times is asked for once.
func parseParams(cmd string) []Param {
	var params []Param
	seen := map[string]bool{}
	for _, m := range paramRe.FindAllStringSubmatch(cmd, -1) {
		if seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		p := Param{Name: m[1], Default: m[2]}
		if choicesRe.MatchString(m[2]) {
			for _, c := range choiceRe.FindAllStringSubmatch(m[2], -1) {
				p.Choices = append(p.Choices, c[1])
			}
			p.Default = p.Choices[0]
		}
		params = append(params, p)
	}
	return params
}

// span is a rune range [start, end) of a string.
type span struct{ start, end int }

// paramSpans returns where the <param> placeholders are in cmd, in runes.
func paramSpans(cmd string) []span {
	var spans []span
	for _, m := range paramRe.FindAllStringIndex(cmd, -1) {
		start := utf8.RuneCountInString(cmd[:m[0]])
		spans = append(spans, span{start, start + utf8.RuneCountInString(cmd[m[0]:m[1]])})
	}
	return spans
}

// filled marks where a parameter's value ended up in a filled command.
type filled struct {
	span
	param int  // index into params
	empty bool // no value: the <name> placeholder was kept
}

// fill replaces every placeholder with its parameter's value. With
// keepEmpty, placeholders without a value stay as <name> (for previews).
func fill(cmd string, params []Param, values []string, keepEmpty bool) (string, []filled) {
	index := map[string]int{}
	for i, p := range params {
		index[p.Name] = i
	}
	var b strings.Builder
	var spans []filled
	runes, last := 0, 0
	for _, m := range paramRe.FindAllStringSubmatchIndex(cmd, -1) {
		b.WriteString(cmd[last:m[0]])
		runes += utf8.RuneCountInString(cmd[last:m[0]])
		last = m[1]

		i, ok := index[cmd[m[2]:m[3]]]
		if !ok {
			b.WriteString(cmd[m[0]:m[1]])
			runes += utf8.RuneCountInString(cmd[m[0]:m[1]])
			continue
		}
		v, empty := values[i], values[i] == ""
		if empty && keepEmpty {
			v = "<" + params[i].Name + ">"
		}
		n := utf8.RuneCountInString(v)
		spans = append(spans, filled{span{runes, runes + n}, i, empty})
		b.WriteString(v)
		runes += n
	}
	b.WriteString(cmd[last:])
	return b.String(), spans
}
