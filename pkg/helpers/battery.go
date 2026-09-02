package helpers

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/movsb/fbui/pkg/power_supply"
)

func WatchPowerStatus(ctx context.Context, callback func(capacity int, charging bool)) {
	reader := power_supply.NewReader("")
	last := -1
	lastCharging := false
	update := func() {
		supplies, err := reader.List()
		if err != nil {
			log.Println("读取电池状态：", err)
		}
		battery, ok := power_supply.FirstBattery(supplies)
		if !ok {
			return
		}
		capacityText, ok := battery.Value("capacity")
		if !ok {
			return
		}
		capacity, err := strconv.Atoi(capacityText)
		if err != nil {
			return
		}
		status, _ := battery.Value("status")
		charging := strings.EqualFold(status, "Charging")
		if capacity == last && charging == lastCharging {
			return
		}
		callback(capacity, charging)
		last = capacity
		lastCharging = charging
	}
	update()
	if err := power_supply.Watch(ctx, update); err == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				update()
			}
		}
	}()
}
