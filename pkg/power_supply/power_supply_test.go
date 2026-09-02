package power_supply

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSupply(t *testing.T, root, name, uevent string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "uevent"), []byte(uevent), 0644); err != nil {
		t.Fatal(err)
	}
}

func writeSupplyType(t *testing.T, root, name, supplyType string) {
	t.Helper()
	path := filepath.Join(root, name, "type")
	if err := os.WriteFile(path, []byte(supplyType+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestReaderListsBatteryBeforeExternalSupplies(t *testing.T) {
	root := t.TempDir()
	writeSupply(t, root, "axp-usb", "POWER_SUPPLY_NAME=axp-usb\nPOWER_SUPPLY_ONLINE=1\nPOWER_SUPPLY_FUTURE_FIELD=value\n")
	writeSupplyType(t, root, "axp-usb", "USB")
	writeSupply(t, root, "axp-battery", "POWER_SUPPLY_NAME=axp-battery\nPOWER_SUPPLY_CAPACITY=97\nPOWER_SUPPLY_STATUS=Charging\n")
	writeSupplyType(t, root, "axp-battery", "Battery")
	supplies, err := NewReader(root).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(supplies) != 2 || supplies[0].Type != "Battery" || supplies[1].Type != "USB" {
		t.Fatalf("supplies=%#v", supplies)
	}
	if value, ok := supplies[1].Value("future_field"); !ok || value != "value" {
		t.Fatalf("unknown field=%q, %v", value, ok)
	}
	battery, ok := FirstBattery(supplies)
	if !ok || battery.Name != "axp-battery" {
		t.Fatalf("battery=%#v, %v", battery, ok)
	}
}

func TestReaderReturnsPartialDataAndError(t *testing.T) {
	root := t.TempDir()
	writeSupply(t, root, "battery", "POWER_SUPPLY_TYPE=Battery\nPOWER_SUPPLY_CAPACITY=50\n")
	if err := os.Mkdir(filepath.Join(root, "broken"), 0755); err != nil {
		t.Fatal(err)
	}
	supplies, err := NewReader(root).List()
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("err=%v", err)
	}
	if len(supplies) != 2 {
		t.Fatalf("supplies=%#v", supplies)
	}
	if _, ok := FirstBattery(supplies); !ok {
		t.Fatal("valid battery was lost")
	}
}

func TestReaderMissingRootIsEmpty(t *testing.T) {
	supplies, err := NewReader(filepath.Join(t.TempDir(), "missing")).List()
	if err != nil || len(supplies) != 0 {
		t.Fatalf("supplies=%#v err=%v", supplies, err)
	}
}

func TestFormatValuesAndPreservesAmbiguousRawUnits(t *testing.T) {
	tests := []struct{ key, value, want string }{
		{"POWER_SUPPLY_TEMP", "363", "36.3 °C (363)"},
		{"POWER_SUPPLY_VOLTAGE_NOW", "4173000", "4.173 V (4173000 µV)"},
		{"POWER_SUPPLY_VOLTAGE_MIN_DESIGN", "3960", "3.960 V (3960, 驱动值疑似 mV)"},
		{"POWER_SUPPLY_TIME_TO_FULL_NOW", "900", "15 分 (900 s)"},
		{"POWER_SUPPLY_STATUS", "Charging", "充电中 (Charging)"},
		{"POWER_SUPPLY_CHARGE_FULL", "3000", "3000"},
		{"POWER_SUPPLY_CONSTANT_CHARGE_CURRENT", "1072", "1072"},
	}
	for _, test := range tests {
		if got := Format(test.key, test.value); got != test.want {
			t.Errorf("Format(%q, %q)=%q want %q", test.key, test.value, got, test.want)
		}
	}
}
