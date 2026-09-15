package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"syscall"
	"time"
)

const executableUpdateInterval = 3 * time.Second

func startExecutableAutoUpdate() context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	path, err := os.Executable()
	if err != nil {
		log.Printf("初始化二进制热更新失败：%v", err)
		return cancel
	}
	info, err := os.Stat(path)
	if err != nil {
		log.Printf("初始化二进制热更新失败：%v", err)
		return cancel
	}
	args := append([]string(nil), os.Args...)
	updater := executableUpdater{
		path:    path,
		modTime: info.ModTime(),
		replace: func() error {
			log.Printf("检测到 FBUI 二进制更新，重新执行：%s", path)
			return syscall.Exec(path, args, os.Environ())
		},
	}
	go func() {
		ticker := time.NewTicker(executableUpdateInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := updater.check(); err != nil {
					log.Printf("二进制热更新失败：%v", err)
				}
			}
		}
	}()
	return cancel
}

type executableUpdater struct {
	path    string
	modTime time.Time
	replace func() error
}

func (u executableUpdater) check() error {
	info, err := os.Stat(u.path)
	if err != nil {
		return fmt.Errorf("检查 FBUI 二进制：%w", err)
	}
	if info.ModTime().Equal(u.modTime) {
		return nil
	}
	// Successful exec never returns. Keep the startup timestamp on failure so
	// the next tick retries rather than marking a failed replacement as applied.
	if err := u.replace(); err != nil {
		return fmt.Errorf("替换 FBUI 进程：%w", err)
	}
	return nil
}
