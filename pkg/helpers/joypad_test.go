package helpers

import (
	"os"
	"testing"
)

func TestJoypadState(t *testing.T) {
	path := t.TempDir() + "/joypad_testmode"
	if err := setJoypadDisabled(path, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("disabled marker was not created: %v", err)
	}
	if err := setJoypadDisabled(path, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("enabled marker still exists: %v", err)
	}
	if err := setJoypadDisabled(path, false); err != nil {
		t.Fatalf("enabling an already enabled joypad: %v", err)
	}
}
