//go:build linux

package osdd

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"strings"
	"time"

	"github.com/movsb/fbui/pkg/disp2"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

type Toast struct {
	Type, Message        string
	Duration, Size, W, H int
	Expires              time.Time `json:"-"`
}

type Renderer struct {
	title, body font.Face
}

func newRenderer(path string) (*Renderer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	parsed, err := opentype.Parse(data)
	if err != nil {
		return nil, err
	}
	title, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: 32, DPI: 72})
	if err != nil {
		return nil, err
	}
	body, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: 24, DPI: 72})
	if err != nil {
		_ = title.Close()
		return nil, err
	}
	return &Renderer{title: title, body: body}, nil
}

func (r *Renderer) close() {
	_ = r.title.Close()
	_ = r.body.Close()
}

var (
	panelColor  = color.NRGBA{R: 17, G: 24, B: 39, A: 255}
	cardColor   = color.NRGBA{R: 31, G: 43, B: 59, A: 255}
	accentColor = color.NRGBA{R: 98, G: 197, B: 255, A: 255}
	valueColor  = color.NRGBA{R: 166, G: 181, B: 203, A: 255}
)

func drawText(back *disp2.Buffer, face font.Face, c color.Color, text string, x, centerY int) {
	metrics := face.Metrics()
	drawer := font.Drawer{Dst: back, Src: image.NewUniform(c), Face: face,
		Dot: fixed.Point26_6{X: fixed.I(x), Y: fixed.I(centerY) + (metrics.Ascent-metrics.Descent)/2}}
	drawer.DrawString(text)
}

func (r *Renderer) draw(overlay *disp2.Overlay, visible bool, statuses [6]string, toast *Toast) error {
	back := overlay.BackBuffer()
	back.Clear()
	bounds := back.Bounds()
	if visible {
		const padding, gap, cellH, headerH = 24, 12, 58, 44
		width := min(640, bounds.Dx()-48)
		height := 2*padding + headerH + 3*(gap+cellH)
		left, top := (bounds.Dx()-width)/2, (bounds.Dy()-height)/2
		back.Fill(image.Rect(left, top, left+width, top+height), panelColor)
		drawText(back, r.title, color.White, "设备状态", left+padding, top+padding+headerH/2)
		hint := "Home / B 关闭"
		drawText(back, r.body, valueColor, hint, left+width-padding-font.MeasureString(r.body, hint).Ceil(), top+padding+headerH/2)
		cellW := (width - 2*padding - gap) / 2
		for i, label := range [6]string{"时间", "电量", "频率", "核心", "温度", "内存"} {
			x, y := left+padding+(i%2)*(cellW+gap), top+padding+headerH+gap+(i/2)*(cellH+gap)
			back.Fill(image.Rect(x, y, x+cellW, y+cellH), cardColor)
			drawText(back, r.body, color.White, label, x+14, y+cellH/2)
			value := statuses[i]
			drawText(back, r.body, valueColor, value, x+cellW-14-font.MeasureString(r.body, value).Ceil(), y+cellH/2)
		}
	}
	if toast != nil {
		text := strings.TrimSpace(toast.Message)
		textW := font.MeasureString(r.body, text).Ceil()
		width, height := min(bounds.Dx(), max(toast.W+toast.Size*150, 300)), min(bounds.Dy(), max(toast.H, 80))
		x, y := (bounds.Dx()-width)/2, (bounds.Dy()-height)/2
		if toast.Type == "volume" {
			var level, maximum int
			if n, _ := fmt.Sscanf(text, "%d / %d", &level, &maximum); n == 2 && maximum > 0 {
				text = fmt.Sprintf("音量：%d%%", min(max(level, 0), maximum)*100/maximum)
			}
			textW = font.MeasureString(r.body, text).Ceil()
			width, height, x, y = textW+38, 52, 16, 16
		}
		frame := image.Rect(x, y, x+width, y+height)
		back.Fill(frame, accentColor)
		back.Fill(frame.Inset(3), panelColor)
		drawText(back, r.body, color.White, text, x+(width-textW)/2, y+height/2)
	}
	return overlay.Present()
}
