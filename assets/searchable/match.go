package searchable

import (
	"bufio"
	"bytes"
	_ "embed"
	"strings"
	"sync"
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

	underlying := [256]byte{}
	buf := underlying[:]

	match := false
	for _, provide := range provides {
		construct(buf, 0, []rune(provide), 0, func(s string) {
			match = subSeq(search, s)
		})
		if match {
			return true
		}
	}

	return false
}

func construct(buf []byte, i int, provide []rune, j int, yield func(string)) {
	if j == len(provide) {
		yield(string(buf[:i]))
		return
	}
	r := provide[j]
	if r < utf8.RuneSelf {
		if i+1 > len(buf) {
			return
		}
		switch {
		case '0' <= r && r <= '9':
			buf[i] = byte(r)
			i++
		case 'a' <= r && r <= 'z':
			buf[i] = byte(r)
			i++
		case 'A' <= r && r <= 'Z':
			r += 0x20
			buf[i] = byte(r)
			i++
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
	construct(buf, i, provide, j+1, yield)
}

func subSeq(a, b string) bool {
	i := 0
	for j := range b {
		if b[j] == a[i] {
			i++
			if i == len(a) {
				return true
			}
		}
	}
	return false
}
