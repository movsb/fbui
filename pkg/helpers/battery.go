package helpers

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mdlayher/kobject"
)

type _BatterInfo struct {
	Capacity       uint8   // 当前电量百分比
	ChargingStatus string  // 充电状态 Charging/Discharging/Full
	Temperature    float32 // 当前温度
}

func readPowerStatus() (*_BatterInfo, error) {
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

	info := _BatterInfo{}

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
func watchKernelObjectEvents(ctx context.Context, callback func(event *kobject.Event)) error {
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

func WatchPowerStatus(ctx context.Context, callback func(capacity int, charging bool)) {
	last := uint8(0)
	lastCharging := false

	update := func() {
		info, err := readPowerStatus()
		if err != nil {
			log.Println(err)
			return
		}

		charging := info.ChargingStatus == `Charging`

		if info.Capacity == last && charging == lastCharging {
			return
		}

		callback(int(info.Capacity), charging)

		last = info.Capacity
		lastCharging = charging
	}

	update()

	go func() {
		// TODO event 里面其实已经有当前的数据了
		if err := watchKernelObjectEvents(context.Background(), func(event *kobject.Event) {
			if event.Subsystem == `power_supply` {
				update()
			}
		}); err != nil {
			for range time.Tick(time.Second * 10) {
				select {
				case <-ctx.Done():
					return
				default:
					update()
				}
			}
		}
	}()
}
