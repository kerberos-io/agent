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

func TestCaptureClearClientsOnlyClearsMatchingRun(t *testing.T) {
	captureDevice := &Capture{}
	oldMain := captureDevice.SetMainClient("rtsp://main/old")
	oldSub := captureDevice.SetSubClient("rtsp://sub/old")
	oldBackchannel := captureDevice.SetBackChannelClient("rtsp://back/old")

	newMain := captureDevice.SetMainClient("rtsp://main/new")
	newSub := captureDevice.SetSubClient("rtsp://sub/new")
	newBackchannel := captureDevice.SetBackChannelClient("rtsp://back/new")

	captureDevice.ClearClients(oldMain, oldSub, oldBackchannel)
	if captureDevice.MainClient() != newMain {
		t.Fatal("stale cleanup cleared the new main client")
	}
	if captureDevice.SubClient() != newSub {
		t.Fatal("stale cleanup cleared the new sub client")
	}

	captureDevice.ClearClients(newMain, newSub, newBackchannel)
	if captureDevice.MainClient() != nil || captureDevice.SubClient() != nil {
		t.Fatal("matching cleanup did not clear current clients")
	}
}
