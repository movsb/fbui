//go:build linux

package osdd

import (
	"encoding/binary"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

const (
	EventKey = 1
	KeyB     = 304
	KeyHome  = 172
)

type InputEvent struct {
	Type, Code uint16
	Value      int32
}

func grabInput(f *os.File, grab int) error {
	const eviocgrab = 0x40044590 // _IOW('E', 0x90, int)
	raw, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var ioctlErr error
	if err := raw.Control(func(fd uintptr) {
		ioctlErr = unix.IoctlSetInt(int(fd), eviocgrab, grab)
	}); err != nil {
		return err
	}
	return ioctlErr
}

func readInput(f *os.File, onEvent func(InputEvent)) error {
	var data [24]byte // ARM64 input_event
	for {
		if _, err := io.ReadFull(f, data[:]); err != nil {
			return err
		}
		onEvent(InputEvent{
			Type:  binary.LittleEndian.Uint16(data[16:18]),
			Code:  binary.LittleEndian.Uint16(data[18:20]),
			Value: int32(binary.LittleEndian.Uint32(data[20:24])),
		})
	}
}
