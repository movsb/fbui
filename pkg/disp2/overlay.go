package disp2

import (
	"errors"
	"image"
)

// ErrUnsupported 表示当前操作系统没有可用的 DISP2 后端。
var ErrUnsupported = errors.New("disp2: only supported on Linux")

// Profile 指定创建图层时采用的硬件配置策略。
type Profile uint8

const (
	// ProfileClone 原样复制 Template，仅修改公开的目标、地址、层级和 alpha。
	ProfileClone Profile = iota
	// ProfileTrimUIOSDReplacement 精确复刻原厂 osdd 对 MainUI 和 OSD 两层的
	// 初始化方式。这个模式用于完全替代原厂进程，不能与原厂 osdd 同时运行。
	ProfileTrimUIOSDReplacement
)

// OverlayOptions 描述一个使用 framebuffer 内存的 DISP2 硬件覆盖层。
type OverlayOptions struct {
	// DisplayPath 和 FramebufferPath 通常分别为 /dev/disp 和 /dev/fb0。
	// 留空时会自动使用这两个默认值。
	DisplayPath     string
	FramebufferPath string

	// Target 是新覆盖层占用的图层。它必须处于未启用状态，本包不会自动抢占
	// 正在使用的图层。
	Target LayerID
	// Template 是用于复制完整配置的已启用图层。复制配置可以保留目前尚未确认
	// 含义的厂商私有字段。TrimUI 上使用原厂 OSD 层作为模板。
	Template LayerID
	// BufferOffsets 是每个绘图缓冲区在 /dev/fb0 中的字节偏移。每个区域必须
	// 未被其他程序占用、按页对齐、能容纳一整帧，而且彼此不能重叠。
	BufferOffsets []int64

	// ZOrder 越大越靠近屏幕最上方。如果它不高于 Template 的当前层级，打开时
	// 会自动调整为 Template.ZOrder+1，确保这个图层确实位于模板之上。
	ZOrder uint8
	// Alpha 指定像素 alpha、全局 alpha 或混合模式。
	Alpha AlphaMode
	// Opacity 是全局不透明度，范围为 0 到 255。
	Opacity uint8
	// Profile 默认为 ProfileClone。只有完全替代原厂 trimui_osdd 时才应选择
	// ProfileTrimUIOSDReplacement。
	Profile Profile
}

// TrimUIOSDReplacementOptions 返回完全替代原厂 trimui_osdd 的配置。它使用
// 原厂面板层 channel 2/layer 0 和原厂双缓冲槽位 6、14 MiB；打开后还会与
// MainUI 图层组成一组配置共同提交。z-order 使用 20 而不是原厂的 8，因为部分
// MainUI 会周期性把自身从 z4 恢复到 z16；20 可以保证恢复瞬间 OSD 仍在上方。
func TrimUIOSDReplacementOptions() OverlayOptions {
	return OverlayOptions{
		DisplayPath:     "/dev/disp",
		FramebufferPath: "/dev/fb0",
		Target:          LayerID{Screen: 0, Channel: 2, Layer: 0},
		Template:        LayerID{Screen: 0, Channel: 1, Layer: 0},
		BufferOffsets:   []int64{6 << 20, 14 << 20},
		ZOrder:          20,
		Alpha:           AlphaMixed,
		Opacity:         255,
		Profile:         ProfileTrimUIOSDReplacement,
	}
}

// TrimUIOverlayOptions 返回已经在当前 TrimUI Tina Linux 固件上验证过的布局。
//
// 原厂 OSD 使用 framebuffer 中 6、10、14、18 MiB 的四个槽位；这里选择随后
// 的 22、26 MiB 作为双缓冲，并使用 channel 2/layer 1、z-order 15。
func TrimUIOverlayOptions() OverlayOptions {
	return OverlayOptions{
		DisplayPath:     "/dev/disp",
		FramebufferPath: "/dev/fb0",
		Target:          LayerID{Screen: 0, Channel: 2, Layer: 1},
		Template:        LayerID{Screen: 0, Channel: 2, Layer: 0},
		BufferOffsets:   []int64{22 << 20, 26 << 20},
		ZOrder:          15,
		Alpha:           AlphaPixel,
		Opacity:         255,
	}
}

// Overlay 管理一个硬件图层和一个或多个 framebuffer 绘图区域。
// Overlay 不是并发安全的，绘制和 Present 应由同一个渲染线程串行执行。
type Overlay struct {
	backend overlayBackend
	buffers []*Buffer
	next    int
	// front 是最近一次成功 Present 的缓冲编号。-1 表示还没有显示过任何帧。
	front int
}

type overlayBackend interface {
	present(index int) error
	hide() error
	close() error
}

// OpenTrimUIOverlay 使用经过验证的 TrimUI 默认布局打开一个覆盖层。
func OpenTrimUIOverlay() (*Overlay, error) { return OpenOverlay(TrimUIOverlayOptions()) }

// BackBuffer 返回下一次 Present 将要显示的缓冲区。
//
// 双缓冲工作方式是：取得 BackBuffer、完成整帧绘制、调用 Present，然后重新
// 取得新的 BackBuffer。不要在 Present 后继续修改原先取得的 Buffer。
func (o *Overlay) BackBuffer() *Buffer { return o.buffers[o.next] }

func (o *Overlay) Bounds() image.Rectangle { return o.buffers[0].Bounds() }

// Present 让 DISP2 显示当前后备缓冲，然后切换到下一个后备缓冲。
func (o *Overlay) Present() error {
	if o == nil || o.backend == nil {
		return errors.New("disp2: overlay is closed")
	}
	if err := o.backend.present(o.next); err != nil {
		return err
	}
	o.front = o.next
	o.next = (o.next + 1) % len(o.buffers)
	return nil
}

// Refresh 重新向 DISP2 提交当前正在显示的缓冲，但不切换缓冲、也不修改像素。
//
// 部分 Tina 固件中的 MainUI 会在每帧提交自身图层时影响整个硬件合成状态，尽管
// DISP_LAYER_GET_CONFIG 读取到的其他图层配置没有变化。常驻 OSD 可以周期性调用
// Refresh，维持自己的图层处于最终提交状态。第一次 Present 之前调用没有效果。
func (o *Overlay) Refresh() error {
	if o == nil || o.backend == nil {
		return errors.New("disp2: overlay is closed")
	}
	if o.front < 0 {
		return nil
	}
	return o.backend.present(o.front)
}

// Hide 隐藏覆盖层，但保留其余有效配置字段。
//
// 厂商驱动收到“除 enable 外全部为零”的配置时，不一定真的关闭图层。因此这里
// 先 GET 当前配置，再只把 enable 改为 0 后 SET 回去。
func (o *Overlay) Hide() error {
	if o == nil || o.backend == nil {
		return nil
	}
	return o.backend.hide()
}

// Close 隐藏图层、解除 framebuffer 映射并关闭设备文件。可重复调用。
// 调用者应当始终 defer Close，并在自行处理的退出信号路径中保证它得到执行。
func (o *Overlay) Close() error {
	if o == nil || o.backend == nil {
		return nil
	}
	err := o.backend.close()
	o.backend, o.buffers = nil, nil
	return err
}
