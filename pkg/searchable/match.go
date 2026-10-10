package searchable

import (
	"bufio"
	"bytes"
	_ "embed"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

//go:embed all.txt
var allTxt []byte

var runeMaps = sync.OnceValue(func() map[rune]string {
	m := map[rune]string{}
	scn := bufio.NewScanner(bytes.NewReader(allTxt))
	for scn.Scan() {
		p := strings.Split(scn.Text(), `:`)
		if len(p) != 2 {
			panic(`数据错误`)
		}
		r := []rune(p[0])[0]
		m[r] = p[1]
	}
	return m
})

// 判断提供的字符串是否和搜索字符串匹配。
//
// search: 来自用户输入的小写首字母序列
// provides: 文件名、翻译名……
func Match(search string, provides ...string) bool {
	if search == "" {
		return true
	}
	search = normalizeQuery(search)
	if search == "" {
		return false
	}

	provides = slices.Compact(provides)

	underlying := [256]byte{}
	buf := underlying[:]

	match := false
	for _, provide := range provides {
		construct(buf, 0, []rune(provide), 0, func(s string) {
			if !match {
				match = subSeq(search, s)
			}
		})
		if match {
			return true
		}
	}

	return false
}

// normalizeQuery makes separators in user input insignificant. Candidate
// names retain hard boundaries such as parentheses so initials cannot match
// across an edition/region qualifier.
func normalizeQuery(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case '0' <= r && r <= '9', 'a' <= r && r <= 'z':
			b.WriteRune(r)
		case 'A' <= r && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		}
	}
	return b.String()
}

func isSoftPunctuation(r rune) bool {
	return strings.ContainsRune("'’‘ʼ-‐‑–—_", r)
}

func isSoftSeparator(s []rune, i int) bool {
	if isSoftPunctuation(s[i]) {
		return true
	}
	if !unicode.IsSpace(s[i]) {
		return false
	}
	// Spaces used merely to pad apostrophes or hyphens are insignificant
	// (for example, "拳皇 '97"). An ordinary word boundary is retained.
	left := i - 1
	for left >= 0 && unicode.IsSpace(s[left]) {
		left--
	}
	right := i + 1
	for right < len(s) && unicode.IsSpace(s[right]) {
		right++
	}
	return left >= 0 && isSoftPunctuation(s[left]) ||
		right < len(s) && isSoftPunctuation(s[right])
}

func appendBoundary(buf []byte, i int) (int, bool) {
	if i > 0 && buf[i-1] == ' ' {
		return i, true
	}
	if i+1 > len(buf) {
		return i, false
	}
	buf[i] = ' '
	return i + 1, true
}

// TODO 支持中途停止
func construct(buf []byte, i int, provide []rune, j int, yield func(string)) {
	if j == len(provide) {
		yield(string(buf[:i]))
		return
	}
	r := provide[j]
	if r < utf8.RuneSelf {
		switch {
		case '0' <= r && r <= '9':
			if i+1 > len(buf) {
				return
			}
			buf[i] = byte(r)
			i++
		case 'a' <= r && r <= 'z':
			if i+1 > len(buf) {
				return
			}
			buf[i] = byte(r)
			i++
		case 'A' <= r && r <= 'Z':
			if i+1 > len(buf) {
				return
			}
			r += 0x20
			buf[i] = byte(r)
			i++
		case !isSoftSeparator(provide, j):
			var ok bool
			i, ok = appendBoundary(buf, i)
			if !ok {
				return
			}
		}
		construct(buf, i, provide, j+1, yield)
		return
	}
	if mapped, ok := runeMaps()[r]; ok {
		parts := strings.SplitSeq(mapped, `,`)
		for part := range parts {
			if i+len(part) > len(buf) {
				return
			}
			copy(buf[i:], []byte(part))
			construct(buf, i+len(part), provide, j+1, yield)
		}
		return
	}

	// 撇号和连字符（包括其两侧的空格）不打断搜索；普通空格、
	// 括号等结构性分隔则保留为边界。
	if !isSoftSeparator(provide, j) {
		var ok bool
		i, ok = appendBoundary(buf, i)
		if !ok {
			return
		}
	}

	construct(buf, i, provide, j+1, yield)
}

// 判断子串。
func subSeq(a, b string) bool {
	return strings.Contains(b, a)
}
