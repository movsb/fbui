package launcher

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunScriptPreservesSetupAndInterceptsRetroArch(t *testing.T) {
	dir := t.TempDir()
	setupPath := filepath.Join(dir, `setup.txt`)
	argsPath := filepath.Join(dir, `args.txt`)
	retroArchPath := filepath.Join(dir, `ra64.trimui`)
	launchPath := filepath.Join(dir, `launch.sh`)
	romPath := filepath.Join(dir, `Roms`, `FC`, `a game.nes`)

	writeExecutable(t, retroArchPath, "#!/bin/sh\nprintf '%s\\n' \"$HOME\" \"$@\" >\"$ARGS_PATH\"\n")
	writeExecutable(t, launchPath, strings.Join([]string{
		`#!/bin/bash`,
		`export HOME="` + dir + `/retro-home"`,
		`export ARGS_PATH="` + argsPath + `"`,
		`printf setup >"` + setupPath + `"`,
		`exec "` + retroArchPath + `" -v -L "` + dir + `/cores/fceumm_libretro.so" "$1"`,
	}, "\n"))

	if err := RunScript(context.Background(), launchPath, romPath); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(setupPath); err != nil || string(got) != `setup` {
		t.Fatalf("setup command: %q, %v", got, err)
	}
	got, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	want := dir + "/retro-home\n-v\n-L\n" + dir + "/cores/fceumm_libretro.so\n" + romPath + "\n"
	if string(got) != want {
		t.Fatalf("RetroArch arguments:\n got %q\nwant %q", got, want)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
}
