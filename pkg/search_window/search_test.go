package search_window

import "testing"

func TestCenteredKeyColumn(t *testing.T) {
	tests := []struct {
		column, from, to, want int
	}{
		{0, 10, 9, 0},
		{9, 10, 9, 8},
		{0, 9, 10, 0},
		{0, 9, 7, 0},
		{4, 9, 7, 3},
		{8, 9, 7, 6},
		{0, 7, 9, 1},
		{6, 7, 9, 7},
	}
	for _, test := range tests {
		if got := centeredKeyColumn(test.column, test.from, test.to); got != test.want {
			t.Errorf("centeredKeyColumn(%d, %d, %d)=%d, want %d", test.column, test.from, test.to, got, test.want)
		}
	}
}

func TestMatchItemsRanksSearchGroupsBeforeNameLength(t *testing.T) {
	items := []Item{
		{Name: `恶魔城`, SearchGroups: [][]string{{`恶魔城`}, nil, {`Majou Densetsu`}}},
		{Name: `摩艾君`, SearchGroups: [][]string{{`摩艾君`}, {`Moai-kun`}, nil}},
	}
	matched := MatchItems(items, `maj`)
	if len(matched) != 2 || matched[0].Name != `摩艾君` || matched[1].Name != `恶魔城` {
		t.Fatalf("unexpected order: %#v", matched)
	}
}
