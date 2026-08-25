package ssh

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/creack/pty"
	"github.com/gliderlabs/ssh"
	"github.com/movsb/fbiw"
	"github.com/movsb/fbui/pkg/helpers"
	"github.com/pkg/sftp"
)

//go:embed id_ed25519
var _privateKey []byte

func Serve(ctx context.Context, callback func(string, error)) {
	ip, err := helpers.GetIP()
	if err != nil {
		callback(``, err)
		return
	}

	var listener net.Listener
	for _, port := range []int{22, 2222, 60022, 0} {
		lis, err := net.Listen(`tcp4`, fmt.Sprintf(`:%d`, port))
		if err != nil {
			log.Println(err)
			continue
		}
		listener = lis
		log.Println(`SSH服务器地址:`, lis)
		break
	}

	if listener == nil {
		callback(``, errors.New(`没有可用的端口。`))
		return
	}

	server := &ssh.Server{
		Handler: handleSession,
		SubsystemHandlers: map[string]ssh.SubsystemHandler{
			`sftp`: handleFiles,
		},
	}

	if err := server.SetOption(
		ssh.HostKeyPEM(_privateKey),
	); err != nil {
		log.Fatal(err)
	}

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	go func() {
		server.Serve(listener)
	}()

	time.AfterFunc(time.Millisecond*500, func() {
		_, port, _ := net.SplitHostPort(listener.Addr().String())
		callback(net.JoinHostPort(ip, port), nil)
	})
}

func home() string {
	return fbiw.Iif(runtime.GOOS == `linux`, `/root`, os.Getenv(`HOME`))
}

func handleSession(s ssh.Session) {
	ptyReq, windowChanges, hasPTY := s.Pty()

	var cmd *exec.Cmd
	if args := s.Command(); len(args) > 0 {
		// exec 请求直接启动程序，不经过 shell。
		cmd = exec.Command(args[0], args[1:]...)
	} else {
		shell := `/bin/sh`
		if sh := os.Getenv(`SHELL`); sh != `` {
			shell = sh
		}
		cmd = exec.Command(shell, `-li`)
	}

	home := home()
	cmd.Env = append(os.Environ(),
		`HOME=`+home,
	)
	cmd.Dir = home

	if !hasPTY {
		runCommand(s, cmd)
		return
	}

	cmd.Env = append(cmd.Env, "TERM="+ptyReq.Term)
	initialSize := &pty.Winsize{
		Cols: uint16(ptyReq.Window.Width),
		Rows: uint16(ptyReq.Window.Height),
	}

	terminal, err := pty.StartWithSize(cmd, initialSize)
	if err != nil {
		fmt.Fprintf(s.Stderr(), "start command: %v\r\n", err)
		_ = s.Exit(1)
		return
	}
	defer terminal.Close()

	// 把客户端后续的窗口大小变化传给本地 PTY。
	go func() {
		for window := range windowChanges {
			_ = pty.Setsize(terminal, &pty.Winsize{
				Cols: uint16(window.Width),
				Rows: uint16(window.Height),
			})
		}
	}()

	// SSH 客户端输入 -> shell PTY
	go func() {
		io.Copy(terminal, s)
	}()

	// shell PTY 输出 -> SSH 客户端
	io.Copy(s, terminal)

	err = cmd.Wait()
	s.Exit(commandExitCode(err))
}

func runCommand(s ssh.Session, cmd *exec.Cmd) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		fmt.Fprintf(s.Stderr(), "open command stdin: %v\n", err)
		_ = s.Exit(255)
		return
	}

	cmd.Stdout = s
	cmd.Stderr = s.Stderr()
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		fmt.Fprintf(s.Stderr(), "start command: %v\n", err)
		_ = s.Exit(255)
		return
	}

	// 不能直接使用 cmd.Stdin = s。os/exec 会等待内部的 stdin 复制
	// goroutine 结束，而 SSH session 要等 s.Exit 才会关闭，两者会循环等待。
	// StdinPipe内部不会主动拷贝，没有新拷贝线程创建，所以不等待Wait()就会结束。
	go func() {
		_, _ = io.Copy(stdin, s)
		_ = stdin.Close()
	}()

	_ = s.Exit(commandExitCode(cmd.Wait()))
}

func commandExitCode(err error) int {
	if err == nil {
		return 0
	}

	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		if code := exitErr.ExitCode(); code >= 0 {
			return code
		}
	}

	return 255
}

func handleFiles(session ssh.Session) {
	server, err := sftp.NewServer(
		session,
		sftp.WithServerWorkingDirectory(home()),
	)
	if err != nil {
		log.Printf("sftp server init error: %s\n", err)
		return
	}
	if err := server.Serve(); err == io.EOF {
		server.Close()
		fmt.Println("sftp client exited session.")
	} else if err != nil {
		fmt.Println("sftp server completed with error:", err)
	}
}
