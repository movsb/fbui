package searchable

import (
	"strings"
	"unicode/utf8"
)

// Prepared stores query-independent transformations and can be serialized to disk.
// Extremely ambiguous names retain a fallback instead of expanding exponentially.
type Prepared struct {
	Length   int
	Literal  string
	Initials []string
	Fallback string
}

func Prepare(name string) Prepared {
	p := Prepared{Length: utf8.RuneCountInString(name), Literal: normalizeLiteral(name)}
	seen := map[string]bool{}
	count := 0
	yield := func(s string) bool {
		count++
		if !seen[s] {
			seen[s] = true
			p.Initials = append(p.Initials, s)
		}
		return count >= 256
	}
	for _, spaces := range []bool{false, true} {
		if constructMode(make([]byte, 256), 0, []rune(name), 0, spaces, yield) {
			p.Initials = nil
			p.Fallback = name
			break
		}
	}
	return p
}

// Query normalizes the user's input once for all entries.
type Query struct {
	literal, initials string
	raw               string
	empty             bool
}

func NewQuery(s string) Query {
	return Query{raw: s, literal: normalizeLiteral(s), initials: normalizeQuery(s), empty: s == ""}
}

func (q Query) Match(p Prepared) bool {
	if q.empty {
		return true
	}
	if q.literal != "" && strings.Contains(p.Literal, q.literal) {
		return true
	}
	if q.initials == "" {
		return false
	}
	if p.Fallback != "" {
		return Match(q.raw, p.Fallback)
	}
	for _, s := range p.Initials {
		if strings.Contains(s, q.initials) {
			return true
		}
	}
	return false
}

// Keys are a conservative candidate filter: every rune of the normalized
// query must occur somewhere in a name's literal or initial variants.
func (p Prepared) Keys() []rune {
	seen := map[rune]bool{}
	for _, s := range append([]string{p.Literal}, p.Initials...) {
		for _, r := range s {
			seen[r] = true
		}
	}
	keys := make([]rune, 0, len(seen))
	for r := range seen {
		keys = append(keys, r)
	}
	return keys
}

func (q Query) Keys() [][]rune {
	var alternatives [][]rune
	for _, s := range []string{q.literal, q.initials} {
		if s != "" {
			alternatives = append(alternatives, []rune(s))
		}
	}
	return alternatives
}
