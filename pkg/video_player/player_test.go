package video_player

import (
	"testing"

	"github.com/movsb/fbiw"
)

func TestContainRectKeepsAbsoluteContainerOffset(t *testing.T) {
	container := fbiw.Rect{X: 320, Y: 120, Width: 640, Height: 480}
	if got, want := containRect(container, 1920, 1080), (fbiw.Rect{X: 320, Y: 180, Width: 640, Height: 360}); got != want {
		t.Fatalf("contain rect = %#v, want %#v", got, want)
	}
}
