package main

import (
	"sort"
	"strings"
	"unicode"
)

// match is a snippet that passed the filter, with the matched rune
// positions in its description, command and tag text.
type match struct {
	idx   int
	score int
	pos   [3][]int
}

// filter keeps the snippets matching every space-separated term of query
// (fuzzy, smart case) and orders them best first. Each term is matched
// against the description, the command and the tags separately.
func filter(snippets []Snippet, query string) []match {
	terms := strings.Fields(query)
	out := make([]match, 0, len(snippets))
	for i, sn := range snippets {
		fields := [3][]rune{[]rune(sn.Description), []rune(sn.Command), []rune(tagText(sn.Tags))}
		m := match{idx: i}
		ok := true
		for _, t := range terms {
			pat := []rune(t)
			caseSensitive := strings.ToLower(t) != t
			best, bestField := 0, -1
			var bestPos []int
			for f, text := range fields {
				if sc, pos, found := fuzzyMatch(pat, text, caseSensitive); found && (bestField < 0 || sc > best) {
					best, bestField, bestPos = sc, f, pos
				}
			}
			if bestField < 0 {
				ok = false
				break
			}
			m.score += best
			m.pos[bestField] = append(m.pos[bestField], bestPos...)
		}
		if ok {
			out = append(out, m)
		}
	}
	if len(terms) > 0 {
		sort.SliceStable(out, func(a, b int) bool { return out[a].score > out[b].score })
	}
	return out
}

// fuzzyMatch finds pat as a subsequence of text. It tries every possible
// start, shrinks each candidate to its tightest window and keeps the best
// scoring one.
func fuzzyMatch(pat, text []rune, caseSensitive bool) (int, []int, bool) {
	if len(pat) == 0 {
		return 0, nil, true
	}
	eq := func(a, b rune) bool {
		if caseSensitive {
			return a == b
		}
		return unicode.ToLower(a) == unicode.ToLower(b)
	}
	best, found := 0, false
	var bestPos []int
	for s := range text {
		if !eq(text[s], pat[0]) {
			continue
		}
		pi, end := 0, -1
		for t := s; t < len(text); t++ {
			if eq(text[t], pat[pi]) {
				if pi++; pi == len(pat) {
					end = t
					break
				}
			}
		}
		if end < 0 {
			break // a later start can't match either
		}
		pos := make([]int, len(pat))
		pi = len(pat) - 1
		for t := end; t >= s && pi >= 0; t-- {
			if eq(text[t], pat[pi]) {
				pos[pi] = t
				pi--
			}
		}
		if sc := score(text, pos); !found || sc > best {
			best, bestPos, found = sc, pos, true
		}
	}
	return best, bestPos, found
}

// score rewards consecutive matches and matches at word starts, and
// penalises gaps.
func score(text []rune, pos []int) int {
	sc := 0
	for i, p := range pos {
		sc += 16
		if p == 0 || isBoundary(text[p-1], text[p]) {
			sc += 10
			if i == 0 {
				sc += 6
			}
		}
		if i > 0 {
			if gap := pos[i] - pos[i-1] - 1; gap == 0 {
				sc += 12
			} else {
				sc -= 3 + min(gap, 15)
			}
		}
	}
	return sc
}

func isBoundary(prev, cur rune) bool {
	if unicode.IsLower(prev) && unicode.IsUpper(cur) {
		return true
	}
	return !unicode.IsLetter(prev) && !unicode.IsDigit(prev)
}
