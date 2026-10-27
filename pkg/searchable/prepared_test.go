package searchable

import (
	"strings"
	"testing"
)

func TestPreparedMatchesOriginal(t *testing.T) {
	names := []string{"拳皇 '97", "重装机兵", "第二次机器人大战", "Final Fantasy VII", "Mitsume ga Tooru", "三（美版", "魂斗罗", "", strings.Repeat("重", 10), strings.Repeat("a", 300)}
	queries := []string{"", " ' - ", "qh97", "qh 97", "QH-97", "zzjb", "czjb", "dec", "d2c", "7", "vii", "Mitsume ga", "mitsumega", "smb", "mb", "魂斗", "hdl", "重", "cc", "aaa", "中文7"}
	for _, name := range names {
		p := Prepare(name)
		for _, query := range queries {
			if got, want := NewQuery(query).Match(p), Match(query, name); got != want {
				t.Errorf("name=%q query=%q got=%v want=%v", name, query, got, want)
			}
		}
	}
}
