package launcher

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// RunScript interprets an emulator launch.sh and intercepts its final
// RetroArch command. Setup commands in the script still run normally.
func RunScript(ctx context.Context, scriptPath, romPath string) error {
	scriptPath, err := filepath.Abs(scriptPath)
	if err != nil {
		return fmt.Errorf(`解析启动脚本路径: %w`, err)
	}
	fp, err := os.Open(scriptPath)
	if err != nil {
		return fmt.Errorf(`打开启动脚本: %w`, err)
	}
	defer fp.Close()

	program, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(fp, scriptPath)
	if err != nil {
		return fmt.Errorf(`解析启动脚本: %w`, err)
	}

	interceptRetroArch := func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(ctx context.Context, args []string) error {
			if !isRetroArchCommand(args) {
				return next(ctx, args)
			}
			hc := interp.HandlerCtx(ctx)
			if runRetroArchCandidates(args, romPath, hc.Env, hc.Dir) {
				return nil
			}
			return interp.ExitStatus(1)
		}
	}

	runner, err := interp.New(
		interp.Dir(filepath.Dir(scriptPath)),
		interp.Env(expand.ListEnviron(os.Environ()...)),
		interp.Params(romPath),
		interp.StdIO(os.Stdin, os.Stdout, os.Stderr),
		interp.ExecHandlers(interceptRetroArch),
	)
	if err != nil {
		return fmt.Errorf(`创建 shell 解释器: %w`, err)
	}
	return runner.Run(ctx, program)
}

func isRetroArchCommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	name := strings.ToLower(filepath.Base(args[0]))
	return strings.Contains(name, `retroarch`) || strings.HasPrefix(name, `ra64.`) || strings.HasPrefix(name, `ra32.`)
}

func runRetroArchCandidates(command []string, romPath string, env expand.Environ, dir string) bool {
	before, originalCore, after := splitByCore(command[1:])
	if originalCore == `` {
		return run(command[0], command[1:], env, dir, `/tmp/retroarch.log`)
	}

	candidates := []string{originalCore}
	coreDir := filepath.Dir(originalCore)
	for name := range rangeDirForEmulators(romPath) {
		candidate := filepath.Join(coreDir, name)
		if candidate != originalCore {
			candidates = append(candidates, candidate)
		}
	}

	for _, core := range candidates {
		logFile := fmt.Sprintf(`/tmp/retroarch_%s.log`, strings.TrimSuffix(filepath.Base(core), `.so`))
		if run(command[0], joinTheCore(before, core, after), env, dir, logFile) {
			log.Println(`成功运行:`, core)
			return true
		}
	}
	return false
}
