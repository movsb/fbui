package power_supply

import (
	"context"
	"log"

	"github.com/mdlayher/kobject"
)

func Watch(ctx context.Context, callback func()) error {
	client, err := kobject.New()
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		_ = client.Close()
	}()
	go func() {
		defer client.Close()
		for {
			event, err := client.Receive()
			if err != nil {
				if ctx.Err() == nil {
					log.Println("监听电源状态：", err)
				}
				return
			}
			select {
			case <-ctx.Done():
				return
			default:
				if event.Subsystem == "power_supply" {
					callback()
				}
			}
		}
	}()
	return nil
}
