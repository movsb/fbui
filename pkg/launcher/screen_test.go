package launcher

import "testing"

func TestHasGrayErrorPixels(t *testing.T) {
	gray := []byte{0x73, 0x75, 0x73, 0xff}
	black := []byte{0, 0, 0, 0}

	pixels := make([]byte, 0, 100*4)
	for range 95 {
		pixels = append(pixels, gray...)
	}
	for range 5 {
		pixels = append(pixels, black...)
	}
	if !hasGrayErrorPixels(pixels) {
		t.Fatal(`95% observed gray pixels should be treated as the error screen`)
	}

	pixels = pixels[:0]
	for range 94 {
		pixels = append(pixels, gray...)
	}
	for range 6 {
		pixels = append(pixels, black...)
	}
	if hasGrayErrorPixels(pixels) {
		t.Fatal(`less than 95% observed gray pixels should not be treated as the error screen`)
	}
	if hasGrayErrorPixels(nil) {
		t.Fatal(`an empty framebuffer should not be treated as the error screen`)
	}
}
