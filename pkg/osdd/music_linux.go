//go:build linux

package osdd

import (
	"os"

	"golang.org/x/sys/unix"
)

// musicKeepalive 只持有 FIFO 写端。最后一个写端关闭后，原厂 musicserver
// 会在 poll 返回 POLLHUP 时空转。这里不发送任何音乐命令。
type musicKeepalive struct {
	writer *os.File
	info   os.FileInfo
}

func (m *musicKeepalive) refresh() {
	const path = "/tmp/trimui_music/widget_cmd"
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		m.close()
		return
	}
	if m.writer != nil && os.SameFile(m.info, info) {
		return
	}
	m.close()
	fd, err := unix.Open(path, unix.O_WRONLY|unix.O_NONBLOCK|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		// 服务尚未打开读端时返回 ENXIO，下次刷新再试。
		return
	}
	writer := os.NewFile(uintptr(fd), path)
	opened, err := writer.Stat()
	if err != nil || !os.SameFile(info, opened) {
		_ = writer.Close()
		return
	}
	m.writer, m.info = writer, opened
}

func (m *musicKeepalive) close() {
	if m.writer != nil {
		_ = m.writer.Close()
		m.writer, m.info = nil, nil
	}
}
