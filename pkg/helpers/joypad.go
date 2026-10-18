package helpers

import (
	"errors"
	"os"
)

const joypadTestModePath = "/tmp/joypad_testmode"

// DisableJoypad 在 fbui 位于前台时关闭 TrimUI 的摇杆采样。fbui 只使用数字
// 按键，因此保留这个标记文件可以避免 trimui_inputd 对摇杆做不必要的轮询。
func DisableJoypad() error {
	return setJoypadDisabled(joypadTestModePath, true)
}

// EnableJoypad 在把前台控制权交给游戏或其他外部程序之前恢复摇杆采样。
func EnableJoypad() error {
	return setJoypadDisabled(joypadTestModePath, false)
}

func setJoypadDisabled(path string, disabled bool) error {
	if !disabled {
		err := os.Remove(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	return file.Close()
}
