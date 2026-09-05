package capture

import (
	"strconv"
	"sync"
	"testing"
)

func TestCaptureClientAccessCanRaceReplacement(t *testing.T) {
	captureDevice := &Capture{}
	captureDevice.SetMainClient("rtsp://main/0")
	captureDevice.SetSubClient("rtsp://sub/0")

	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		for replacement := 1; replacement <= 1000; replacement++ {
			suffix := strconv.Itoa(replacement)
			captureDevice.SetMainClient("rtsp://main/" + suffix)
			captureDevice.SetSubClient("rtsp://sub/" + suffix)
		}
	}()
	go func() {
		defer workers.Done()
		for snapshot := 0; snapshot < 1000; snapshot++ {
			if captureDevice.MainClient() == nil {
				t.Error("MainClient() returned nil")
				return
			}
			if captureDevice.SubClient() == nil {
				t.Error("SubClient() returned nil")
				return
			}
		}
	}()
	workers.Wait()
}
