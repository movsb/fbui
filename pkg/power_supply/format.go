package power_supply

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

var labels = map[string]string{
	"POWER_SUPPLY_NAME":                    "名称",
	"POWER_SUPPLY_TYPE":                    "类型",
	"POWER_SUPPLY_PRESENT":                 "设备存在",
	"POWER_SUPPLY_ONLINE":                  "已接通",
	"POWER_SUPPLY_STATUS":                  "状态",
	"POWER_SUPPLY_HEALTH":                  "健康度",
	"POWER_SUPPLY_CAPACITY":                "电量",
	"POWER_SUPPLY_CAPACITY_LEVEL":          "电量等级",
	"POWER_SUPPLY_VOLTAGE_NOW":             "当前电压",
	"POWER_SUPPLY_VOLTAGE_MIN_DESIGN":      "设计最低电压",
	"POWER_SUPPLY_VOLTAGE_MAX_DESIGN":      "设计最高电压",
	"POWER_SUPPLY_CURRENT_NOW":             "当前电流",
	"POWER_SUPPLY_CURRENT_AVG":             "平均电流",
	"POWER_SUPPLY_POWER_NOW":               "当前功率",
	"POWER_SUPPLY_CHARGE_NOW":              "当前电荷量",
	"POWER_SUPPLY_CHARGE_COUNTER":          "电荷计数",
	"POWER_SUPPLY_CHARGE_FULL":             "满充电荷量",
	"POWER_SUPPLY_CHARGE_FULL_DESIGN":      "设计满充电荷量",
	"POWER_SUPPLY_ENERGY_NOW":              "当前能量",
	"POWER_SUPPLY_ENERGY_FULL":             "满充能量",
	"POWER_SUPPLY_ENERGY_FULL_DESIGN":      "设计满充能量",
	"POWER_SUPPLY_CONSTANT_CHARGE_CURRENT": "恒定充电电流",
	"POWER_SUPPLY_INPUT_CURRENT_LIMIT":     "输入电流限制",
	"POWER_SUPPLY_TEMP":                    "温度",
	"POWER_SUPPLY_TEMP_ALERT_MIN":          "最低温度告警",
	"POWER_SUPPLY_TEMP_ALERT_MAX":          "最高温度告警",
	"POWER_SUPPLY_TEMP_AMBIENT":            "环境温度",
	"POWER_SUPPLY_TEMP_AMBIENT_ALERT_MIN":  "最低环境温度告警",
	"POWER_SUPPLY_TEMP_AMBIENT_ALERT_MAX":  "最高环境温度告警",
	"POWER_SUPPLY_TIME_TO_EMPTY_NOW":       "预计耗尽时间",
	"POWER_SUPPLY_TIME_TO_FULL_NOW":        "预计充满时间",
	"POWER_SUPPLY_MANUFACTURER":            "制造商",
	"POWER_SUPPLY_MODEL_NAME":              "型号",
	"POWER_SUPPLY_SERIAL_NUMBER":           "序列号",
	"POWER_SUPPLY_TECHNOLOGY":              "电池技术",
	"POWER_SUPPLY_CYCLE_COUNT":             "循环次数",
	"POWER_SUPPLY_CAPACITY_ALERT_MIN":      "最低电量告警",
}

func Label(key string) string {
	if label, ok := labels[key]; ok {
		return label
	}
	return strings.TrimPrefix(key, "POWER_SUPPLY_")
}

func Format(key, value string) string {
	switch key {
	case "POWER_SUPPLY_PRESENT", "POWER_SUPPLY_ONLINE":
		if value == "1" {
			return "是 (1)"
		}
		if value == "0" {
			return "否 (0)"
		}
	case "POWER_SUPPLY_CAPACITY", "POWER_SUPPLY_CAPACITY_ALERT_MIN":
		return value + "%"
	case "POWER_SUPPLY_TEMP", "POWER_SUPPLY_TEMP_ALERT_MIN", "POWER_SUPPLY_TEMP_ALERT_MAX",
		"POWER_SUPPLY_TEMP_AMBIENT", "POWER_SUPPLY_TEMP_AMBIENT_ALERT_MIN", "POWER_SUPPLY_TEMP_AMBIENT_ALERT_MAX":
		if n, err := strconv.ParseFloat(value, 64); err == nil {
			return fmt.Sprintf("%.1f °C (%s)", n/10, value)
		}
	case "POWER_SUPPLY_VOLTAGE_NOW", "POWER_SUPPLY_VOLTAGE_MIN_DESIGN", "POWER_SUPPLY_VOLTAGE_MAX_DESIGN":
		if n, err := strconv.ParseFloat(value, 64); err == nil {
			// Some vendor drivers expose mV despite the sysfs ABI specifying µV.
			if n != 0 && n < 100000 {
				return fmt.Sprintf("%.3f V (%s, 驱动值疑似 mV)", n/1000, value)
			}
			return fmt.Sprintf("%.3f V (%s µV)", n/1e6, value)
		}
	case "POWER_SUPPLY_TIME_TO_EMPTY_NOW", "POWER_SUPPLY_TIME_TO_FULL_NOW":
		if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
			return formatDuration(time.Duration(seconds)*time.Second) + " (" + value + " s)"
		}
	case "POWER_SUPPLY_STATUS", "POWER_SUPPLY_HEALTH", "POWER_SUPPLY_CAPACITY_LEVEL", "POWER_SUPPLY_TYPE":
		return translate(value) + " (" + value + ")"
	}
	return value
}

func formatDuration(value time.Duration) string {
	hours := int(value / time.Hour)
	minutes := int(value % time.Hour / time.Minute)
	if hours > 0 {
		return fmt.Sprintf("%d 小时 %d 分", hours, minutes)
	}
	return fmt.Sprintf("%d 分", minutes)
}

func translate(value string) string {
	translations := map[string]string{
		"Battery":      "电池",
		"USB":          "USB 供电",
		"Mains":        "交流供电",
		"Charging":     "充电中",
		"Discharging":  "放电中",
		"Full":         "已充满",
		"Not charging": "未充电",
		"Unknown":      "未知",
		"Good":         "良好",
		"Overheat":     "过热",
		"Dead":         "失效",
		"Over voltage": "过压",
		"High":         "高",
		"Normal":       "正常",
		"Low":          "低",
		"Critical":     "严重不足",
	}
	if translated, ok := translations[value]; ok {
		return translated
	}
	return value
}
