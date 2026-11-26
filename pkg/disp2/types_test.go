package disp2

import (
	"encoding/binary"
	"image"
	"image/color"
	"testing"
)

func TestRawLayerConfigOffsets(t *testing.T) {
	var c rawLayerConfig
	c.setID(LayerID{Channel: 2, Layer: 1})
	c[configEnableOffset] = 1
	c[configZOrderOffset] = 15
	c[configAlphaModeOffset] = byte(AlphaPixel)
	c[configAlphaValueOffset] = 255
	negativeX := int32(-3)
	binary.LittleEndian.PutUint32(c[configScreenXOffset:], uint32(negativeX))
	binary.LittleEndian.PutUint32(c[configScreenYOffset:], 4)
	binary.LittleEndian.PutUint32(c[configScreenWOffset:], 1024)
	binary.LittleEndian.PutUint32(c[configScreenHOffset:], 768)
	binary.LittleEndian.PutUint64(c[configAddressOffset:], 0xfd600000)

	got := c.info(0)
	if got.ID != (LayerID{Screen: 0, Channel: 2, Layer: 1}) || !got.Enabled || got.ZOrder != 15 || got.Address != 0xfd600000 {
		t.Fatalf("unexpected layer: %+v", got)
	}
	if got.Screen != image.Rect(-3, 4, 1021, 772) {
		t.Fatalf("unexpected rectangle: %v", got.Screen)
	}
}

func TestBufferUsesDISPByteOrderAndClips(t *testing.T) {
	b := Buffer{Pix: make([]byte, 3*2*4), Width: 3, Height: 2, Stride: 12}
	b.Fill(image.Rect(-1, 0, 2, 1), color.NRGBA{R: 1, G: 2, B: 3, A: 4})
	if got := b.Pix[:8]; string(got) != string([]byte{3, 2, 1, 4, 3, 2, 1, 4}) {
		t.Fatalf("unexpected bytes: %v", got)
	}
	if got := color.NRGBAModel.Convert(b.At(1, 0)).(color.NRGBA); got != (color.NRGBA{R: 1, G: 2, B: 3, A: 4}) {
		t.Fatalf("unexpected color: %v", got)
	}
	b.Clear()
	for _, value := range b.Pix {
		if value != 0 {
			t.Fatal("clear left non-zero bytes")
		}
	}
}
