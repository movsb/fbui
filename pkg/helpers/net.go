package helpers

import (
	"errors"
	"net"
	"runtime"

	"github.com/movsb/fbiw"
)

var errNoIP = errors.New("未获取到IP地址，Wi-Fi启动了吗？")

func GetIP() (string, error) {
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
