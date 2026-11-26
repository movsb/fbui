//go:build !linux

package disp2

// OpenOverlay 在非 Linux 平台始终返回 ErrUnsupported。保留这个实现是为了让
// 使用 fbui 的桌面开发环境仍然可以编译和运行不涉及硬件的测试。
func OpenOverlay(OverlayOptions) (*Overlay, error) { return nil, ErrUnsupported }
