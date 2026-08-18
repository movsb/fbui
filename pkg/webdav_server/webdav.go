package webdav_server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"runtime"
	"time"

	"github.com/movsb/fbiw"
	"golang.org/x/net/webdav"
)

// callback会在服务器准备好后于线程中调用。
func _NewWebDAVServer(ctx context.Context, dir string, callback func(string, error)) {
	lanIP, err := getIP()
	if err != nil {
		callback(``, err)
		return
	}

	ctx, cancel := context.WithCancel(ctx)

	server := webdav.Handler{
		FileSystem: webdav.Dir(dir),
		LockSystem: webdav.NewMemLS(),
	}

	lis, err := net.Listen(`tcp4`, `0.0.0.0:6666`)
	if err != nil {
		callback(``, err)
		log.Println(err)
		cancel()
		return
	}

	go func() {
		<-ctx.Done()
		lis.Close()
	}()

	go func() {
		defer log.Println(`文件服务器退出`)
		http.Serve(lis, &server)
	}()

	go func() {
		now := time.Now()
		for time.Since(now) < time.Second*3 {
			conn, err := net.Dial(`tcp4`, `localhost:6666`)
			if err != nil {
				time.Sleep(time.Millisecond * 500)
				continue
			}
			time.Sleep(time.Millisecond * 500)
			conn.Close()
			callback(fmt.Sprintf(`http://%s:6666`, lanIP), nil)
			return
		}
		callback(``, errors.New(`启动超时`))
		cancel()
	}()
}

var errNoIP = errors.New("未获取到IP地址，Wi-Fi启动了吗？")

func getIP() (string, error) {
	face, err := net.InterfaceByName(fbiw.Iif(runtime.GOOS == `linux`, `wlan0`, `en0`))
	if err != nil {
		return ``, err
	}

	if face.Flags&net.FlagUp == 0 {
		return ``, errNoIP
	}
	if face.Flags&net.FlagRunning == 0 {
		return ``, errNoIP
	}

	addrs, err := face.Addrs()
	if err != nil {
		return ``, err
	}

	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipnet.IP.To4()
		if ip != nil {
			return ip.String(), nil
		}
	}

	return "", errNoIP
}
