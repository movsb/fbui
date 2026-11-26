//go:build linux

package disp2

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	// DISP_LAYER_SET_CONFIG 和 DISP_LAYER_GET_CONFIG 是 Tina DISP2 的原始
	// ioctl 命令号。它们不是 Linux 主线 DRM API。
	dispLayerSetConfig = 0x47
	dispLayerGetConfig = 0x48

	// fbdev 用这两个 ioctl 返回物理显存地址、长度、分辨率和像素位数。
	fbGetVScreenInfo = 0x4600
	fbGetFScreenInfo = 0x4602
)

// linuxOverlay 保存真正持有系统资源的 Linux 后端。公开的 Overlay 只负责
// 缓冲轮换，所有设备文件和 mmap 生命周期都集中在这里管理。
type linuxOverlay struct {
	disp, fb *os.File
	mapping  []byte
	config   rawLayerConfig
	// companions 会和 config 在同一次 SET_CONFIG 中提交。替代原厂 osdd 时
	// 它包含规范化后的 MainUI 图层，普通 ProfileClone 模式下为空。
	companions []rawLayerConfig
	id         LayerID
	addresses  []uint64
	closed     bool
}

// ioctl 是一个很薄的系统调用包装。使用 unsafe.Pointer 是因为 DISP2 ABI 要求
// 传入厂商定义的数据结构地址，而 x/sys/unix 没有对应的高级封装。
func ioctl(fd uintptr, request uintptr, arg unsafe.Pointer) error {
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, fd, request, uintptr(arg))
	if errno != 0 {
		return errno
	}
	return nil
}

// layerIOCTL 构造 DISP2 驱动要求的 unsigned long args[4]：
//
//	args[0] = screen 编号
//	args[1] = disp_layer_config 数组地址
//	args[2] = 数组元素数量（这里固定为 1）
//	args[3] = 保留
//
// 在 ARM64 上 unsigned long 和 Go 的 uintptr 都是 64 位，因此这个布局与
// 驱动一致。
func layerIOCTL(f *os.File, request uintptr, screen uint32, config *rawLayerConfig) error {
	args := [4]uintptr{uintptr(screen), uintptr(unsafe.Pointer(config)), 1, 0}
	return ioctl(f.Fd(), request, unsafe.Pointer(&args[0]))
}

// setLayers 在一次 ioctl 中提交一组连续的 disp_layer_config。原厂 osdd 会把
// MainUI 与自己的 OSD 层作为一个合成状态维护，替代模式必须保持这一语义。
func setLayers(f *os.File, screen uint32, configs []rawLayerConfig) error {
	if len(configs) == 0 {
		return nil
	}
	args := [4]uintptr{uintptr(screen), uintptr(unsafe.Pointer(&configs[0])), uintptr(len(configs)), 0}
	return ioctl(f.Fd(), dispLayerSetConfig, unsafe.Pointer(&args[0]))
}

// getLayer 先把 channel/layer 选择器写入结构体尾部，再让驱动填充完整配置。
func getLayer(f *os.File, id LayerID) (rawLayerConfig, error) {
	var c rawLayerConfig
	c.setID(id)
	err := layerIOCTL(f, dispLayerGetConfig, id.Screen, &c)
	return c, err
}

// OpenOverlay 打开设备、检查 framebuffer 布局、读取模板图层并映射绘图内存。
// 它只准备配置，不会立刻启用 Target；第一次 Present 才会让图层出现在屏幕上。
func OpenOverlay(options OverlayOptions) (_ *Overlay, resultErr error) {
	// 设备路径允许覆盖，方便在测试环境或设备节点名称不同的固件中使用。
	if options.DisplayPath == "" {
		options.DisplayPath = "/dev/disp"
	}
	if options.FramebufferPath == "" {
		options.FramebufferPath = "/dev/fb0"
	}
	if len(options.BufferOffsets) == 0 {
		return nil, errors.New("disp2: no framebuffer offsets supplied")
	}
	if options.Target == options.Template {
		return nil, errors.New("disp2: target and template layer must differ")
	}

	// 使用具名返回错误配合 defer，确保初始化在任意中间步骤失败时，前面已经
	// 打开的文件和建立的映射都会被回收。
	disp, err := os.OpenFile(options.DisplayPath, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("disp2: open display: %w", err)
	}
	defer func() {
		if resultErr != nil {
			disp.Close()
		}
	}()
	fb, err := os.OpenFile(options.FramebufferPath, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("disp2: open framebuffer: %w", err)
	}
	defer func() {
		if resultErr != nil {
			fb.Close()
		}
	}()

	// 不声明 fb_fix_screeninfo/fb_var_screeninfo Go struct，避免 Go/C 对齐规则
	// 不一致。以下大小和字段偏移均对应当前 ARM64 Linux fbdev ABI。
	var fix [80]byte
	if err := ioctl(fb.Fd(), fbGetFScreenInfo, unsafe.Pointer(&fix[0])); err != nil {
		return nil, fmt.Errorf("disp2: get fixed framebuffer info: %w", err)
	}
	varInfo := [160]byte{}
	if err := ioctl(fb.Fd(), fbGetVScreenInfo, unsafe.Pointer(&varInfo[0])); err != nil {
		return nil, fmt.Errorf("disp2: get variable framebuffer info: %w", err)
	}
	physical := binary.LittleEndian.Uint64(fix[16:])
	length := uint64(binary.LittleEndian.Uint32(fix[24:]))
	width := int(binary.LittleEndian.Uint32(varInfo[0:]))
	height := int(binary.LittleEndian.Uint32(varInfo[4:]))
	bpp := binary.LittleEndian.Uint32(varInfo[24:])
	// 当前绘图和字节序实现只支持真机验证过的 32 位 ARGB8888 framebuffer。
	if physical == 0 || length == 0 || width <= 0 || height <= 0 || bpp != 32 {
		return nil, fmt.Errorf("disp2: unsupported framebuffer: address=%#x length=%d size=%dx%d bpp=%d", physical, length, width, height, bpp)
	}
	frameBytes := uint64(width * height * 4)

	// 先检查目标层，宁可返回错误也不覆盖 osdd 或其他进程正在使用的图层。
	target, err := getLayer(disp, options.Target)
	if err != nil {
		return nil, fmt.Errorf("disp2: get target layer: %w", err)
	}
	if target.info(options.Target.Screen).Enabled && options.Profile != ProfileTrimUIOSDReplacement {
		return nil, fmt.Errorf("disp2: target layer %s is already enabled", options.Target)
	}
	// 从工作中的原厂图层复制配置，可以保留色彩空间、crop、扫描方式等尚未
	// 完全确认的字段。这里只允许使用已启用且尺寸与 fb0 相同的模板。
	template, err := getLayer(disp, options.Template)
	if err != nil {
		return nil, fmt.Errorf("disp2: get template layer: %w", err)
	}
	ti := template.info(options.Template.Screen)
	if !ti.Enabled {
		return nil, fmt.Errorf("disp2: template layer %s is disabled", options.Template)
	}
	if ti.Screen.Dx() != width || ti.Screen.Dy() != height {
		return nil, fmt.Errorf("disp2: template is %dx%d, framebuffer is %dx%d", ti.Screen.Dx(), ti.Screen.Dy(), width, height)
	}
	// “覆盖层”的层级必须严格高于模板。不同 MainUI/固件会动态选择不同的
	// z-order（真机上先后观察到过 4 和 16），因此不能只依赖默认常量 15。
	// 保留调用者更高的显式值；值不足时自动提升到模板上一层。
	zOrder := options.ZOrder
	if options.Profile != ProfileTrimUIOSDReplacement && zOrder <= ti.ZOrder {
		if ti.ZOrder == 255 {
			return nil, fmt.Errorf("disp2: template layer %s already uses maximum z-order", options.Template)
		}
		zOrder = ti.ZOrder + 1
	}

	// 把调用者给出的 fb0 文件偏移换算成 DISP2 需要的物理地址，同时检查每个
	// 缓冲都能容纳一帧且彼此不重叠。
	addresses := make([]uint64, len(options.BufferOffsets))
	for i, offset := range options.BufferOffsets {
		if offset < 0 || uint64(offset)%uint64(os.Getpagesize()) != 0 {
			return nil, fmt.Errorf("disp2: buffer %d offset %#x is not page aligned", i, offset)
		}
		end := uint64(offset) + frameBytes
		if end < uint64(offset) || end > length {
			return nil, fmt.Errorf("disp2: buffer %d [%#x,%#x) exceeds framebuffer length %#x", i, offset, end, length)
		}
		addresses[i] = physical + uint64(offset)
		for j := 0; j < i; j++ {
			other := uint64(options.BufferOffsets[j])
			if uint64(offset) < other+frameBytes && other < end {
				return nil, fmt.Errorf("disp2: buffers %d and %d overlap", j, i)
			}
		}
	}
	// 映射整个 fb0 后再按偏移切片。MAP_SHARED 保证 CPU 写入对显示引擎可见。
	mapping, err := unix.Mmap(int(fb.Fd()), 0, int(length), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("disp2: map framebuffer: %w", err)
	}
	defer func() {
		if resultErr != nil {
			unix.Munmap(mapping)
		}
	}()
	buffers := make([]*Buffer, len(options.BufferOffsets))
	for i, offset := range options.BufferOffsets {
		pix := mapping[int(offset):int(uint64(offset)+frameBytes)]
		buffers[i] = &Buffer{Pix: pix, Width: width, Height: height, Stride: width * 4}
		buffers[i].Clear()
	}

	// 复制模板后只改经过验证的字段：目标选择器、层级、alpha 和 enable。
	// 首次 Present 时才会把实际缓冲物理地址写入 configAddressOffset。
	config := template
	var companions []rawLayerConfig
	if options.Profile == ProfileTrimUIOSDReplacement {
		// 原厂 trimui_osdd 初始化后 MainUI 的实际配置为 z4、mixed alpha/255。
		// MainUI 后续可能重新提交 z16/global alpha，因此这个 companion 会在
		// 每次 Present/Refresh 时与 OSD 一起重提。
		mainLayer := template
		mainLayer[configZOrderOffset] = 4
		mainLayer[configAlphaModeOffset] = byte(AlphaMixed)
		mainLayer[configAlphaValueOffset] = 255
		companions = append(companions, mainLayer)

		// 原厂进程被 SIGKILL 后，消息层 channel 3/layer 0 可能保持启用。替代
		// 模式已经在 channel 2 上同时绘制面板和 toast，应把残留消息层关闭。
		messageID := LayerID{Screen: options.Target.Screen, Channel: 3, Layer: 0}
		if messageLayer, err := getLayer(disp, messageID); err == nil && messageLayer.info(messageID.Screen).Enabled {
			messageLayer[configEnableOffset] = 0
			companions = append(companions, messageLayer)
		}

		// MainUI 当前可能正在显示虚拟 framebuffer 的第二页，crop.x/y 中会带
		// 有页偏移。原厂 OSD 的 crop 起点固定为零，并使用 ARGB8888 格式 0。
		clear(config[0x60:0x64])
		clear(config[0x80:0x88])
	}
	config.setID(options.Target)
	config[configZOrderOffset] = zOrder
	config[configAlphaModeOffset] = byte(options.Alpha)
	config[configAlphaValueOffset] = options.Opacity
	config[configEnableOffset] = 0
	b := &linuxOverlay{disp: disp, fb: fb, mapping: mapping, config: config, companions: companions, id: options.Target, addresses: addresses}
	return &Overlay{backend: b, buffers: buffers, front: -1}, nil
}

// present 更新当前 framebuffer 物理地址并启用图层。切换地址就是 DISP2 上的
// page flip；不需要把像素从后备缓冲复制到原厂主 framebuffer。
func (o *linuxOverlay) present(index int) error {
	if o.closed {
		return errors.New("disp2: overlay is closed")
	}
	binary.LittleEndian.PutUint64(o.config[configAddressOffset:], o.addresses[index])
	o.config[configEnableOffset] = 1
	configs := append(append(make([]rawLayerConfig, 0, len(o.companions)+1), o.companions...), o.config)
	if err := setLayers(o.disp, o.id.Screen, configs); err != nil {
		return fmt.Errorf("disp2: present: %w", err)
	}
	return nil
}

// hide 必须基于驱动刚刚返回的有效配置关闭图层。实测直接 SET 一个全零配置，
// 即使 ioctl 返回成功也可能让图层继续显示。
func (o *linuxOverlay) hide() error {
	if o.closed {
		return nil
	}
	current, err := getLayer(o.disp, o.id)
	if err != nil {
		return fmt.Errorf("disp2: get layer before hiding: %w", err)
	}
	current[configEnableOffset] = 0
	configs := append(append(make([]rawLayerConfig, 0, len(o.companions)+1), o.companions...), current)
	if err := setLayers(o.disp, o.id.Screen, configs); err != nil {
		return fmt.Errorf("disp2: hide: %w", err)
	}
	return nil
}

// close 即使 Hide 失败也会继续回收 mmap 和文件描述符，并返回遇到的第一个错误。
func (o *linuxOverlay) close() error {
	if o.closed {
		return nil
	}
	var result error
	if err := o.hide(); err != nil {
		result = err
	}
	o.closed = true
	if err := unix.Munmap(o.mapping); err != nil && result == nil {
		result = err
	}
	if err := o.fb.Close(); err != nil && result == nil {
		result = err
	}
	if err := o.disp.Close(); err != nil && result == nil {
		result = err
	}
	return result
}
