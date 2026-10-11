package searchable

import (
	"reflect"
	"testing"
)

func TestConstruct(t *testing.T) {
	buf := make([]byte, 256)
	out := []string{}
	construct(buf, 0, []rune(`1《重aB,`), 0, func(s string) {
		out = append(out, s)
	})
	if !reflect.DeepEqual(out, []string{`1 zab `, `1 cab `}) {
		t.Fatalf(`不相等: %s`, out)
	}
}

func TestConstruct2(t *testing.T) {
	buf := make([]byte, 256)
	construct(buf, 0, []rune(`重装机兵.nes`), 0, func(s string) {
		t.Log(s)
	})
}

func TestMatchIgnoresSeparatorDifferences(t *testing.T) {
	for _, query := range []string{`qh97`, `qh 97`, `qh '97`, `QH-97`} {
		if !Match(query, `拳皇 '97`) {
			t.Errorf("query %q did not match", query)
		}
	}
}

func TestMatchSeparatorOnlyQueryDoesNotMatchEverything(t *testing.T) {
	if Match(` ' - `, `拳皇 '97`) {
		t.Fatal("separator-only query unexpectedly matched")
	}
}

func TestMatchDoesNotCrossStructuralSeparator(t *testing.T) {
	if Match(`smb`, `三（美版`) {
		t.Fatal(`smb unexpectedly matched 三（美版`)
	}
	if Match(`smb`, `s mb`) {
		t.Fatal(`smb unexpectedly matched s mb`)
	}
	if !Match(`mb`, `三（美版`) {
		t.Fatal(`mb did not match 美版`)
	}
}

func TestConstructCompressesSpaces(t *testing.T) {
	buf := make([]byte, 256)
	var got string
	construct(buf, 0, []rune(`s   mb`), 0, func(s string) { got = s })
	if got != `s mb` {
		t.Fatalf("got %q, want %q", got, `s mb`)
	}
	if Match(`smb`, `s   mb`) {
		t.Fatal(`smb unexpectedly matched s   mb`)
	}
	if !Match(`qh97`, `拳皇   '97`) {
		t.Fatal(`qh97 did not match 拳皇   '97`)
	}
}

func TestMatchChineseDigitAlternatives(t *testing.T) {
	for _, query := range []string{`dec`, `d2c`} {
		if !Match(query, `第二次机器人大战`) {
			t.Errorf("query %q did not match", query)
		}
	}
}

func TestMatchMultiWordLatinName(t *testing.T) {
	if !Match(`Mitsume ga`, `Mitsume ga Tooru`) {
		t.Fatal(`Mitsume ga did not match Mitsume ga Tooru`)
	}
	if !Match(`Mitsume   ga`, `Mitsume ga Tooru`) {
		t.Fatal(`repeated query spaces were not compressed`)
	}
	if Match(`Mitsumega`, `Mitsume ga Tooru`) {
		t.Fatal(`Mitsumega unexpectedly crossed a word boundary`)
	}
}
