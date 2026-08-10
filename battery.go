package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mdlayher/kobject"
)

type BatterInfo struct {
	Capacity       uint8   // 当前电量百分比
	ChargingStatus string  // 充电状态 Charging/Discharging/Full
	Temperature    float32 // 当前温度
}

func ReadPowerStatus() (*BatterInfo, error) {
	paths, err := filepath.Glob(`/sys/class/power_supply/*/type`)
	if err != nil {
		return nil, err
	}
	var batteryPath string
	for _, path := range paths {
		ty, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		ts := strings.ToLower(strings.TrimSpace(string(ty)))
		if ts == `battery` {
			batteryPath = path
			break
		}
	}
	if batteryPath == `` {
		return nil, fmt.Errorf(`没找到电池目录`)
	}

	dir, _ := filepath.Split(batteryPath)
	uevent := filepath.Join(dir, `uevent`)

	fp, err := os.Open(uevent)
	if err != nil {
		return nil, err
	}
	defer fp.Close()

	info := BatterInfo{}

	scn := bufio.NewScanner(fp)
	for scn.Scan() {
		key, value, ok := strings.Cut(scn.Text(), `=`)
		if !ok {
			continue
		}
		switch key {
		case `POWER_SUPPLY_CAPACITY`:
			n, _ := strconv.Atoi(value)
			info.Capacity = uint8(n)
		case `POWER_SUPPLY_STATUS`:
			info.ChargingStatus = value
		case `POWER_SUPPLY_TEMP`:
			n, _ := strconv.Atoi(value)
			info.Temperature = float32(n) / 10
		}
	}
	if scn.Err() != nil {
		return nil, scn.Err()
	}

	return &info, nil
}

// 监听netlink事件。回调发生在线程中。
func WatchKernelObjectEvents(ctx context.Context, callback func(event *kobject.Event)) error {
	client, err := kobject.New()
	if err != nil {
		log.Println(err)
		return err
	}

	go func() {
		defer client.Close()

		for {
			event, err := client.Receive()
			if err != nil {
				log.Println(err)
				break
			}
			select {
			case <-ctx.Done():
				return
			default:
				callback(event)
			}
		}
	}()

	return nil
}
