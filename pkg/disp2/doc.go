// Package disp2 封装了 Allwinner/Tina Linux 的 DISP2 硬件图层接口。
//
// 这个包刻意采用保守策略：创建新覆盖层时，先读取并复制一个已经正常工作的
// 原厂图层配置，再仅修改经过 TrimUI 真机验证的字段。这样可以保留厂商驱动所需、
// 但尚未完全公开或确认的其余结构体字段。
//
// 本包面向当前测试过的 TrimUI Tina Linux 固件，不是通用的 Linux DRM、fbdev
// 或任意 Allwinner 设备抽象。
package disp2
