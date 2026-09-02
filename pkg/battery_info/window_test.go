package battery_info

import (
	"strings"
	"testing"

	"github.com/movsb/fbui/pkg/power_supply"
)

func TestBuildRowsIncludesKnownAndRawFields(t *testing.T) {
	supplies := []power_supply.Supply{{Name: "bat0", Type: "Battery", Values: map[string]string{
		"POWER_SUPPLY_NAME": "bat0", "POWER_SUPPLY_TYPE": "Battery", "POWER_SUPPLY_CAPACITY": "88", "POWER_SUPPLY_UNKNOWN": "raw",
	}}}
	rows := buildRows(supplies)
	var text string
	for _, row := range rows {
		text += row.label + "=" + row.value + "\n"
	}
	for _, want := range []string{"bat0", "概览", "电量=88%", "原始数据", "POWER_SUPPLY_UNKNOWN=raw"} {
		if !strings.Contains(text, want) {
			t.Errorf("rows missing %q:\n%s", want, text)
		}
	}
}

func TestBuildSummaryHandlesBatteryAndExternalOnly(t *testing.T) {
	battery := power_supply.Supply{Name: "bat0", Type: "Battery", Values: map[string]string{
		"POWER_SUPPLY_CAPACITY": "97", "POWER_SUPPLY_STATUS": "Charging", "POWER_SUPPLY_TEMP": "363",
	}}
	got := buildSummary([]power_supply.Supply{battery})
	for _, want := range []string{"电量：97%", "状态：充电中 (Charging)", "温度：36.3 °C (363)"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary missing %q: %s", want, got)
		}
	}
	external := power_supply.Supply{Name: "usb", Type: "USB", Values: map[string]string{}}
	if got := buildSummary([]power_supply.Supply{external}); got != "未发现电池 · 已发现 1 个外部供电设备" {
		t.Fatalf("summary=%q", got)
	}
}
