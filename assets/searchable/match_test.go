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
	if !reflect.DeepEqual(out, []string{`1zhongab`, `1chongab`, `1tongab`}) {
		t.Fatalf(`不相等: %s`, out)
	}
}

func TestConstruct2(t *testing.T) {
	buf := make([]byte, 256)
	construct(buf, 0, []rune(`重装机兵.nes`), 0, func(s string) {
		t.Log(s)
	})
}
