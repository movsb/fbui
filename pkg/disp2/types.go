package disp2

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
)

const (
	// disp_layer_config 最后一个有效字段位于 176..180，但结构体内部含 uint64，
	// ARM64 C ABI 会把 sizeof 向 8 字节对齐到 184。末尾四字节是 padding；单个
	// ioctl 时误用 180 往往不易察觉，配置数组从第二项开始却会立即错位。
	layerConfigSize = 184

	// 以下偏移来自对原厂程序和真机 ioctl 数据的交叉验证。
	configModeOffset       = 0x00
	configZOrderOffset     = 0x04
	configAlphaModeOffset  = 0x05
	configAlphaValueOffset = 0x06
	configScreenXOffset    = 0x08
	configScreenYOffset    = 0x0c
	configScreenWOffset    = 0x10
	configScreenHOffset    = 0x14
	configAddressOffset    = 0x20
	configFormatOffset     = 0x60
	configEnableOffset     = 0xa8
	configChannelOffset    = 0xac
	configLayerOffset      = 0xb0
)

// LayerID 唯一标识一个 DISP2 硬件图层。
//
// Screen 是显示输出编号；Channel 是硬件通道；Layer 是通道内部的图层编号。
// 当前固件中每个 channel 包含四个 layer。
type LayerID struct {
	Screen  uint32
	Channel uint32
	Layer   uint32
}

func (id LayerID) String() string {
	return fmt.Sprintf("screen %d, channel %d, layer %d", id.Screen, id.Channel, id.Layer)
}

// AlphaMode 决定硬件如何使用像素 alpha 和全局 alpha。
type AlphaMode uint8

const (
	// AlphaPixel 只使用 ARGB8888 像素自身携带的 alpha。
	AlphaPixel AlphaMode = 0
	// AlphaGlobal 使用整个图层统一的 Opacity。
	AlphaGlobal AlphaMode = 1
	// AlphaMixed 同时使用像素 alpha 和全局 Opacity。
	AlphaMixed AlphaMode = 2
)

// LayerInfo 是 disp_layer_config 中已经确认含义的稳定字段集合。
// 在测试过的 Tina Linux 固件上，Format 为 0 表示 ARGB8888。
type LayerInfo struct {
	ID      LayerID
	Enabled bool
	Mode    uint32
	ZOrder  uint8
	Alpha   AlphaMode
	Opacity uint8
	Screen  image.Rectangle
	Address uint64
	Format  uint32
}

type rawLayerConfig [layerConfigSize]byte

// setID 写入 GET/SET_CONFIG 都会使用的图层选择器。
func (c *rawLayerConfig) setID(id LayerID) {
	binary.LittleEndian.PutUint32(c[configChannelOffset:], id.Channel)
	binary.LittleEndian.PutUint32(c[configLayerOffset:], id.Layer)
}

// info 将原始字节转换为不依赖 C 结构体对齐规则的公开信息。
func (c *rawLayerConfig) info(screen uint32) LayerInfo {
	x := int(int32(binary.LittleEndian.Uint32(c[configScreenXOffset:])))
	y := int(int32(binary.LittleEndian.Uint32(c[configScreenYOffset:])))
	w := int(binary.LittleEndian.Uint32(c[configScreenWOffset:]))
	h := int(binary.LittleEndian.Uint32(c[configScreenHOffset:]))
	return LayerInfo{
		ID: LayerID{
			Screen:  screen,
			Channel: binary.LittleEndian.Uint32(c[configChannelOffset:]),
			Layer:   binary.LittleEndian.Uint32(c[configLayerOffset:]),
		},
		Enabled: c[configEnableOffset] != 0,
		Mode:    binary.LittleEndian.Uint32(c[configModeOffset:]),
		ZOrder:  c[configZOrderOffset],
		Alpha:   AlphaMode(c[configAlphaModeOffset]),
		Opacity: c[configAlphaValueOffset],
		Screen:  image.Rect(x, y, x+w, y+h),
		Address: binary.LittleEndian.Uint64(c[configAddressOffset:]),
		Format:  binary.LittleEndian.Uint32(c[configFormatOffset:]),
	}
}

// Buffer 是由 mmap 映射出来的 ARGB8888 绘图表面，并实现 image/draw 使用的
// draw.Image 接口。
//
// Pix 的内存字节顺序是 BGRA，而不是 Go 中常见的 RGBA。这是因为 DISP2 的格式
// 名称描述的是 32 位整数中的 ARGB 排列；该整数在小端 ARM 机器内存中依次存成
// B、G、R、A。一般应通过 Set、Fill 或 image/draw 绘制，只有追求性能时才直接
// 操作 Pix。
type Buffer struct {
	Pix           []byte
	Width, Height int
	Stride        int
}

func (b *Buffer) Bounds() image.Rectangle { return image.Rect(0, 0, b.Width, b.Height) }
func (b *Buffer) ColorModel() color.Model { return color.NRGBAModel }

// At 读取一个像素并转换成 Go 的 NRGBA 表示。越界读取返回全透明黑色。
func (b *Buffer) At(x, y int) color.Color {
	if !(image.Point{X: x, Y: y}.In(b.Bounds())) {
		return color.NRGBA{}
	}
	i := y*b.Stride + x*4
	return color.NRGBA{R: b.Pix[i+2], G: b.Pix[i+1], B: b.Pix[i], A: b.Pix[i+3]}
}

// Set 写入一个像素。输入颜色会转换为非预乘的 NRGBA，越界写入会被忽略。
func (b *Buffer) Set(x, y int, value color.Color) {
	if !(image.Point{X: x, Y: y}.In(b.Bounds())) {
		return
	}
	c := color.NRGBAModel.Convert(value).(color.NRGBA)
	i := y*b.Stride + x*4
	b.Pix[i], b.Pix[i+1], b.Pix[i+2], b.Pix[i+3] = c.B, c.G, c.R, c.A
}

// Fill 用同一种颜色填充矩形区域。矩形会自动裁剪到 Buffer 的有效范围内。
func (b *Buffer) Fill(r image.Rectangle, value color.Color) {
	r = r.Intersect(b.Bounds())
	if r.Empty() {
		return
	}
	c := color.NRGBAModel.Convert(value).(color.NRGBA)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := b.Pix[y*b.Stride+r.Min.X*4 : y*b.Stride+r.Max.X*4]
		for i := 0; i < len(row); i += 4 {
			row[i], row[i+1], row[i+2], row[i+3] = c.B, c.G, c.R, c.A
		}
	}
}

// Clear 将整张缓冲清为全透明黑色。
func (b *Buffer) Clear() { clear(b.Pix) }
