package launcher

import (
	"bufio"
	"bytes"
	"fmt"
	"iter"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"mvdan.cc/sh/v3/expand"
)

// HOME=$RA_DIR/ $RA_DIR/ra64.trimui -v $NET_PARAM -L $RA_DIR/.retroarch/cores/mame2010_libretro.so "$*"
func splitByCore(options []string) (beforeCore []string, theCore string, afterCore []string) {
	current := &beforeCore

	for i := 0; i < len(options); i++ {
		switch opt := options[i]; opt {
		case `-L`:
			*current = append(*current, opt)
			if i+1 >= len(options) {
				return
			}
			theCore = options[i+1]
			i++
			current = &afterCore
		default:
			*current = append(*current, opt)
		}
	}

	return
}

func joinTheCore(before []string, core string, after []string) (out []string) {
	out = append(out, before...)
	out = append(out, core)
	out = append(out, after...)
	return
}

// 从最后一级往上找看看所属目录像哪种平台/模拟器（因为包含子目录，所以要简单遍历一下。
//
// 输入：/mnt/SDCARD/Roms/FC/smb.nes
// 输出：[fceumm_libretro.so, nestopia_libretro.so, ...]
//
// 有可能是：/mnt/SDCARD/Emus/FBNEO/../../Roms/FBNEO/dinods.zip
// 需要清理一下，否则会重复执行。
func rangeDirForEmulators(romPath string) iter.Seq[string] {
	romDir, _ := filepath.Split(filepath.Clean(romPath))
	romDirParts := strings.Split(romDir, `/`)
	slices.Reverse(romDirParts)

	return func(yield func(string) bool) {
		for _, component := range romDirParts {
			component = strings.ToLower(component)
			if strings.HasPrefix(component, `cps`) {
				component = `arcade`
			}
			if strings.HasPrefix(component, `mame`) {
				component = `arcade`
			}
			switch component {
			case `fbneo`, `neogeo`:
				component = `arcade`
			}
			if emus, found := emuMaps[component]; found {
				for _, emu := range emus {
					so := emu + `_libretro.so`
					if !yield(so) {
						return
					}
				}
			}
		}
	}
}

// 运行指定的模拟器，返回是否“运行成功”。
//
// 目前只通过判断日志内来判断，所以可能不准确。
func run(binary string, args []string, env expand.Environ, dir, logFile string) bool {
	log.Println(`Running:`, args)

	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Env = exportedEnvironment(env)
	tmpFile, err := os.Create(logFile)
	if err != nil {
		log.Panicln(err)
	}

	cmd.Stdout = tmpFile
	cmd.Stderr = tmpFile

	var hasScreenError atomic.Bool

	if err := cmd.Start(); err != nil {
		log.Println(`Start error:`, err)
		fmt.Fprintln(tmpFile, err)
	} else {
		// FBNEO系列会在失败时显示错误画面，导致模拟失败也不会退出。
		// 所以开个子进程监听日志报错信息，如果发现是此种情况，则直接Kill掉。
		var terminated atomic.Bool
		go func() {
			shouldKill := false
			for range 5 {
				if terminated.Load() {
					break
				}

				all, _ := os.ReadFile(logFile)
				hasScreenError.Store(hasFinalBurnErrorScreen())
				if hasFinalBurnErrorLogs(all) || hasScreenError.Load() {
					// log.Println(`文件包含FB错误或错误屏幕：`, logFile)
					// 温和杀，cmd.Process.Kill 太暴力了。
					// TODO 不一定能杀掉，如果杀不掉，强杀。
					// log.Println(`Klll: SIGTERM`)
					if syscall.Kill(cmd.Process.Pid, syscall.SIGTERM) == nil {
						break
					}
					shouldKill = true
				}

				time.Sleep(time.Second * 1)
			}
			// 前面没杀掉，暴力杀。
			if shouldKill {
				// log.Println(`Klll: SIGKILL`)
				syscall.Kill(cmd.Process.Pid, syscall.SIGKILL)
			}
		}()

		if err := cmd.Wait(); err != nil {
			// 即便退出码不为0也认为成功，因为RetroArch退出码不稳定。
			// 出错不要返回，后面通过日志判断。
			fmt.Fprintln(tmpFile, err)
		}

		terminated.Store(true)
	}

	tmpFile.Close()

	// 如果有屏幕错误，不需要再判断日志了。
	if hasScreenError.Load() {
		return false
	}

	logContent, _ := os.ReadFile(logFile)
	contains := func(s string) bool {
		return bytes.Contains(logContent, []byte(s))
	}

	// 这个错是通用的。
	if contains(`Cannot load this game`) {
		return false
	}
	if contains(`[INFO] [Core] Unloading core`) {
		return false
	}
	if contains(`[ERROR] [Content] 无法加载内容。`) {
		return false
	}
	if contains(`[ERROR] [Content]: 加载游戏失败`) {
		return false
	}
	if contains(`--libretro argument`) && contains(`is not a file, core name or directory`) {
		log.Println(`找不到模拟器核心：`, args)
		return false
	}

	// 这个错误是 FBNEO（或类似？）的报出来的。
	// MAME 可能也有类似的，但是我还没测试到。
	if hasFinalBurnErrorLogs(logContent) {
		return false
	}

	// MAME 的
	if contains(`Fatal error: Required files are missing`) {
		return false
	}

	if contains(`signal: segmentation fault`) {
		return false
	}

	return true
}

func exportedEnvironment(env expand.Environ) []string {
	var out []string
	env.Each(func(name string, variable expand.Variable) bool {
		if variable.IsSet() && variable.Exported && variable.Kind == expand.String {
			out = append(out, name+`=`+variable.String())
		}
		return true
	})
	return out
}

// 硬盘平台名 -> 能打开的模拟器列表。
//
// 暂时也包含一些模拟器名，待去除。
var emuMaps = map[string][]string{
	`arcade`: {
		`fbneo`,
		`fbalpha2012_neogeo`,
		`fbalpha`,
		`fbalpha2012`,
		`mamearcade`,
		`mame2010`,
		`mame2003_plus`,
		`fbalpha2012_cps1`,
		`fbalpha2012_cps2`,
		`fbalpha2012_cps3`,
	},
	`atari2600`: {
		`stella`,
	},
	`fc`: {
		`fceumm`,
		`nestopia`,
		`retro8`,
	},
	`gb`: {
		`gambatte`,
		`gambatte_gb`,
	},
	`gba`: {
		`mgba`,
		`gpsp`,
		`vba_next`,
		`vbam`,
		`mednafen_gba`,
	},
	`gbc`: {
		`gambatte`,
	},
	`md`: {
		`picodrive`,
		`genesis_plus_gx`,
		`genesis_plus_gx_wide`,
	},
	`sfc`: {
		`snes9x2005`,
		`snes9x`,
		`snes9x2005_plus`,
		`snes9x2010`,
	},
}

func hasFinalBurnErrorLogs(logs []byte) bool {
	contains := func(s ...string) bool {
		for _, x := range s {
			if !bytes.Contains(logs, []byte(x)) {
				return false
			}
		}
		return true
	}

	if contains(`was successfully started`) {
		return false
	}

	// fbneo + captcomm.zip
	if contains(`ROM at index`, `is required`) {
		return true
	}

	// 本身缺少并非不能启动，可能从父ROM那里来。
	// 示例：fbalpha2012_cps1 + dino.zip
	// contains(`ROM index`, `was not found`)

	return false
}

// 截屏并检测是不是其“蓝屏”画面。
//
// 简单检测手段：前面全部是[64 64 64 ff]，后面是[ff ff ff ff]
func hasFinalBurnErrorScreen() bool {
	defer func() { recover() }()

	fp, err := os.Open(`/dev/fb0`)
	if err != nil {
		return false
	}
	defer fp.Close()

	buf := bufio.NewReader(fp)

	// 是否成功读取并相等
	skip := func(b, g, r, a byte) {
		n := 0
		for {
			t, err := buf.Peek(4)
			if err != nil {
				panic(err)
			}
			if t[0] == b && t[1] == g && t[2] == r && t[3] == a {
				buf.Discard(4)
				n++
				continue
			}
			break
		}
		if n <= 0 {
			panic(`skip not enough`)
		}
	}

	skip(0x64, 0x64, 0x64, 0xFF)
	skip(0xFF, 0xFF, 0xFF, 0xFF)
	skip(0x69, 0x69, 0x69, 0xFF)

	return true
}
