package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/movsb/fbiw"
	"github.com/movsb/fbui/pkg/config"
)

const (
	systemUILaunchEnv = "FBUI_SYSTEM_UI"
	systemUIHookName  = "fbui-system-ui.sh"
	stockMainUI       = "/usr/trimui/bin/MainUI"
)

func systemUIHookPath() string {
	return filepath.Join(config.SDCARDRoot, "System", "starts", systemUIHookName)
}

func (w *MainWindow) showSystemUISetting() {
	path := systemUIHookPath()
	_, err := os.Stat(path)
	enabled := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		w.showSystemUIError(err)
		return
	}

	description, action := "当前已关闭。开启后，下次开机进入 FBUI。退出 FBUI 后回到原版系统界面。", "开启"
	if enabled {
		description, action = "当前已开启。关闭后，下次开机进入原版系统界面。", "关闭"
	}
	w.app.ShowAlertDialog(w.doc, fbiw.AlertDialogOptions{
		Title:       "替换系统界面（重启生效）",
		Description: description,
		ActionText:  action,
		CancelText:  "取消",
		OnAction: func() {
			if err := setSystemUIReplacement(path, !enabled); err != nil {
				w.showSystemUIError(err)
			}
		},
	})
}

func (w *MainWindow) showSystemUIError(err error) {
	w.app.ShowAlertDialog(w.doc, fbiw.AlertDialogOptions{
		Title:       "无法更新系统界面设置",
		Description: err.Error(),
		ActionText:  "确定",
	})
}

func setSystemUIReplacement(path string, enabled bool) error {
	if !enabled {
		err := os.Remove(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	if runtime.GOOS != "linux" {
		return errors.New("此功能仅支持使用内置 MainUI 的 TrimUI 固件")
	}
	for _, alternate := range []string{filepath.Join(config.SDCARDRoot, "trimui"), "/mnt/UDISK/trimui.img"} {
		if _, err := os.Stat(alternate); !errors.Is(err, os.ErrNotExist) {
			if err != nil {
				return err
			}
			return errors.New("检测到其他系统启动入口，此功能需要使用内置 MainUI")
		}
	}
	startup, err := os.ReadFile("/usr/trimui/bin/runtrimui.sh")
	if err != nil {
		return err
	}
	if !strings.Contains(string(startup), "System/starts") ||
		!strings.Contains(string(startup), "/usr/trimui/bin:$PATH") ||
		!strings.Contains(string(startup), "\n          MainUI\n") {
		return errors.New("无法识别当前固件的 MainUI 启动入口")
	}
	if _, err := os.Stat(stockMainUI); err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".fbui-system-ui-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()

	if _, err := file.WriteString(systemUIHook(executable)); err != nil {
		return err
	}
	if err := file.Chmod(0o755); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func systemUIHook(executable string) string {
	quotedExecutable := "'" + strings.ReplaceAll(executable, "'", "'\\''") + "'"
	return fmt.Sprintf(`#!/bin/sh
mainui=%s
fbui=%s

# runtrimui calls this file once, then starts MainUI after initializing the device.
if [ "$0" != "$mainui" ]; then
    [ -x "$fbui" ] && [ -x "$mainui" ] || exit 0
    [ ! -d /mnt/SDCARD/trimui ] && [ ! -e /mnt/UDISK/trimui.img ] || exit 0
    mount -o bind "$0" "$mainui"
    exit $?
fi

# Detach the temporary mount while this shell still has the script open.
# Future launches by the firmware go directly to the original MainUI.
umount -l "$mainui" || exit 1
if [ -x "$fbui" ]; then
    # The firmware starts wpa_supplicant. Stock MainUI starts DHCP separately.
    dhcp_pid=
    if [ "$(/usr/trimui/bin/systemval wifi)" = 1 ] &&
       ! ip -4 addr show dev wlan0 scope global | grep -q 'inet '; then
        dhcp_running=
        for pid in $(pidof udhcpc); do
            if tr '\000' '\n' < "/proc/$pid/cmdline" 2>/dev/null | grep -qx wlan0; then
                dhcp_running=1
                break
            fi
        done
        if [ -z "$dhcp_running" ]; then
            udhcpc -f -i wlan0 &
            dhcp_pid=$!
        fi
    fi
    (
        cd "${fbui%%/*}" || exit 1
        %s=1 "$fbui"
    )
    if [ -n "$dhcp_pid" ]; then
        kill "$dhcp_pid" 2>/dev/null
        wait "$dhcp_pid" 2>/dev/null
    fi
    rm -f /tmp/joypad_testmode
fi
exec "$mainui" "$@"
`, stockMainUI, quotedExecutable, systemUILaunchEnv)
}
