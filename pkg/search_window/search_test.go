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
