package helpers

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"
)

type _CPUStat struct {
	User      uint64
	Nice      uint64
	System    uint64
	Idle      uint64
	IOWait    uint64
	IRQ       uint64
	SoftIRQ   uint64
	Steal     uint64
	Guest     uint64
	GuestNice uint64
}

func readCPUStat() (_CPUStat, error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return _CPUStat{}, err
	}

	var stat _CPUStat

	_, err = fmt.Sscanf(
		string(data),
		"cpu %d %d %d %d %d %d %d %d %d %d",
		&stat.User,
		&stat.Nice,
		&stat.System,
		&stat.Idle,
		&stat.IOWait,
		&stat.IRQ,
		&stat.SoftIRQ,
		&stat.Steal,
		&stat.Guest,
		&stat.GuestNice,
	)

	return stat, err
}

func cpuUsage(prev, curr _CPUStat) float64 {
	prevIdle := prev.Idle + prev.IOWait
	currIdle := curr.Idle + curr.IOWait

	prevTotal :=
		prev.User +
			prev.Nice +
			prev.System +
			prev.Idle +
			prev.IOWait +
			prev.IRQ +
			prev.SoftIRQ +
			prev.Steal

	currTotal :=
		curr.User +
			curr.Nice +
			curr.System +
			curr.Idle +
			curr.IOWait +
			curr.IRQ +
			curr.SoftIRQ +
			curr.Steal

	totalDelta := currTotal - prevTotal
	idleDelta := currIdle - prevIdle

	if totalDelta == 0 {
		return 0
	}

	return float64(totalDelta-idleDelta) / float64(totalDelta)
}

// 周期地监测CPU占用率。
//
// 输出字符串格式：[50/400%]。
// 前者表示当前使用，后者表示总CPU数。
func WatchCPU(ctx context.Context, interval time.Duration, callback func(s string)) {
	prev, err := readCPUStat()
	if err != nil {
		log.Println(`读CPU状态失败:`, err)
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second * 2):
			curr, err := readCPUStat()
			if err != nil {
				log.Println(`读CPU状态失败:`, err)
				break
			}
			usage := cpuUsage(prev, curr)
			callback(fmt.Sprintf(`[%d/%d%%]`, int(usage*float64(runtime.NumCPU())*100), runtime.NumCPU()*100))
			prev = curr
		}
	}
}
