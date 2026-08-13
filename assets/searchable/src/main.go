package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/movsb/fbiw"
)

//go:generate go run .

func chinese() map[rune]string {
	rsp := fbiw.Must1(http.Get(`https://github.com/ervinzhao/hanzipinyin/raw/refs/heads/master/data/hzpy-utf8.txt`))
	defer rsp.Body.Close()

	r := bufio.NewReader(rsp.Body)
	if bom, err := r.Peek(3); err == nil && bytes.Equal(bom, []byte{0xEF, 0xBB, 0xBF}) {
		r.Discard(3)
	}

	m := map[rune]string{}

	scn := bufio.NewScanner(r)
	for scn.Scan() {
		p := strings.Split(scn.Text(), `,`)
		if len(p) != 6 {
			log.Println(`错误的行:`, scn.Text())
			continue
		}
		char := []rune(p[0])[0]
		pinyin := p[1]
		initial := pinyin[:1]
		if existed, ok := m[char]; ok {
			m[char] = existed + `,` + initial
		} else {
			m[char] = initial
		}
	}
	if scn.Err() != nil {
		log.Fatalln(scn.Err())
	}

	return m
}

// TODO 没有作任何优化。
// 汉字是共享拼音的，拼音也就那么几十上百个，完全不需要重复。
func main() {
	out := fbiw.Must1(os.Create(`../all.txt`))
	defer out.Close()
	ch := chinese()
	for r, p := range ch {
		fmt.Fprintf(out, "%c:%s\n", r, strings.ToLower(p))
	}
}
