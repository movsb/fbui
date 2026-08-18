package game_names

import "testing"

func TestName(t *testing.T) {
	n := Translate(`1942`)
	if n != `1942 (修订版 B)` {
		t.Fatal(`名字不对`)
	}
}
