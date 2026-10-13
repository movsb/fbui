package launcher

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
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

func TestRetroArchCoreCandidates(t *testing.T) {
	coreDir := filepath.Join(`root`, `cores`)
	core := func(name string) string { return filepath.Join(coreDir, name+`_libretro.so`) }

	tests := []struct {
		name       string
		original   string
		scriptPath string
		want       []string
	}{
		{
			name:       `non-arcade keeps original first`,
			original:   core(`nestopia`),
			scriptPath: filepath.Join(`root`, `Emus`, `FC`, `launch.sh`),
			want:       []string{core(`nestopia`), core(`fceumm`), core(`retro8`)},
		},
		{
			name:       `arcade uses fixed order and deduplicates original`,
			original:   core(`mame2010`),
			scriptPath: filepath.Join(`root`, `Emus`, `ARCADE`, `launch.sh`),
			want: []string{
				core(`fbneo`), core(`mamearcade`), core(`mame2010`), core(`mame2003_plus`),
				core(`fbalpha2012_neogeo`), core(`fbalpha`), core(`fbalpha2012`),
				core(`fbalpha2012_cps1`), core(`fbalpha2012_cps2`), core(`fbalpha2012_cps3`),
			},
		},
		{
			name:       `arcade keeps unknown original as last resort`,
			original:   core(`custom_arcade`),
			scriptPath: filepath.Join(`root`, `Emus`, `FBNEO`, `launch.sh`),
			want: []string{
				core(`fbneo`), core(`mamearcade`), core(`mame2010`), core(`mame2003_plus`),
				core(`fbalpha2012_neogeo`), core(`fbalpha`), core(`fbalpha2012`),
				core(`fbalpha2012_cps1`), core(`fbalpha2012_cps2`), core(`fbalpha2012_cps3`),
				core(`custom_arcade`),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := retroArchCoreCandidates(test.original, test.scriptPath); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("candidates:\n got %q\nwant %q", got, test.want)
			}
		})
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
}
