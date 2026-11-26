//go:build linux

package osdd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type stockProcess struct {
	path, dir string
	args, env []string
}

func stockOSDPath(path string) bool {
	switch path {
	case "/usr/trimui/osd/trimui_osdd", "/mnt/SDCARD/trimui/osd/trimui_osdd", "/mnt/UDISK/trimui/osd/trimui_osdd":
		return true
	}
	return false
}

func procExecutable(pid int) (string, error) {
	return os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "exe"))
}

func findStockOSD() (int, string, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, "", err
	}
	var pid int
	var path string
	for _, entry := range entries {
		id, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		exe, err := procExecutable(id)
		if err != nil || !stockOSDPath(exe) {
			continue
		}
		if pid != 0 {
			return 0, "", errors.New("检测到多个原厂 OSD 进程")
		}
		pid, path = id, exe
	}
	return pid, path, nil
}

func stopStockOSD(ctx context.Context) (*stockProcess, error) {
	pid, path, err := findStockOSD()
	if err != nil || pid == 0 {
		return nil, err
	}
	proc := filepath.Join("/proc", strconv.Itoa(pid))
	dir, err := os.Readlink(filepath.Join(proc, "cwd"))
	if err != nil {
		return nil, err
	}
	readList := func(name string) ([]string, error) {
		data, err := os.ReadFile(filepath.Join(proc, name))
		if err != nil {
			return nil, err
		}
		if len(data) == 0 {
			return []string{}, nil
		}
		return strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00"), nil
	}
	args, err := readList("cmdline")
	if err != nil || len(args) == 0 {
		return nil, fmt.Errorf("无法保存原厂 OSD 的启动参数")
	}
	env, err := readList("environ")
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if exe, _ := procExecutable(pid); exe != path {
		return nil, fmt.Errorf("原厂 OSD 已更换，取消接管")
	}
	if err := unix.Kill(pid, unix.SIGTERM); err != nil {
		return nil, err
	}
	// 发出信号后先确认原进程退出，再决定是否接管或恢复。
	snapshot := &stockProcess{path: path, dir: dir, args: args, env: env}
	exited := func() bool {
		if exe, _ := procExecutable(pid); exe == path {
			return false
		}
		// 热更新保留 PID，原厂 OSD 可能是上一个程序映像启动的子进程。
		_, _ = unix.Wait4(pid, nil, unix.WNOHANG, nil)
		return true
	}
	for range 60 {
		if exited() {
			return snapshot, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	// 某些固件的 OSD 会忽略 SIGTERM。只对原先确认的原厂进程升级信号。
	if exited() {
		return snapshot, nil
	}
	if err := unix.Kill(pid, unix.SIGKILL); err != nil && !errors.Is(err, unix.ESRCH) {
		return nil, err
	}
	for range 20 {
		if exited() {
			return snapshot, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil, fmt.Errorf("原厂 OSD 未退出")
}

func (p *stockProcess) restore() error {
	if p == nil {
		return nil
	}
	pid, _, err := findStockOSD()
	if err != nil || pid != 0 {
		return err
	}
	cmd := &exec.Cmd{
		Path: p.path, Args: p.args, Dir: p.dir, Env: p.env,
		SysProcAttr: &syscall.SysProcAttr{Setsid: true},
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
