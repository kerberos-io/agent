package models

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCommunicationRunChannelDispatchAfterClose(t *testing.T) {
	communication := &Communication{}
	run := NewAgentRun(context.Background(), communication, false)
	if err := run.Activate(); err != nil {
		t.Fatal(err)
	}

	if !communication.TrySendLiveHDHandshake(LiveHDHandshake{}) {
		t.Fatal("TrySendLiveHDHandshake() rejected an available channel")
	}
	if !communication.TrySendMotion(MotionDataPartial{}) {
		t.Fatal("TrySendMotion() rejected an available channel")
	}
	if !communication.TrySendONVIF(OnvifAction{}) {
		t.Fatal("TrySendONVIF() rejected an available channel")
	}

	report := run.Shutdown(context.Background(), errors.New("test complete"))
	if !report.Complete {
		t.Fatalf("shutdown report = %+v, want complete", report)
	}
	if communication.TrySendLiveHDHandshake(LiveHDHandshake{}) {
		t.Fatal("TrySendLiveHDHandshake() accepted a closed run")
	}
	if communication.TrySendMotion(MotionDataPartial{}) {
		t.Fatal("TrySendMotion() accepted a closed run")
	}
	if communication.TrySendONVIF(OnvifAction{}) {
		t.Fatal("TrySendONVIF() accepted a closed run")
	}
}

func TestCommunicationDispatchCanRaceRunChannelClose(t *testing.T) {
	communication := &Communication{}
	run := NewAgentRun(context.Background(), communication, false)
	if err := run.Activate(); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	var senders sync.WaitGroup
	for sender := 0; sender < 20; sender++ {
		senders.Add(1)
		go func() {
			defer senders.Done()
			<-start
			for attempt := 0; attempt < 100; attempt++ {
				communication.TrySendLiveHDHandshake(LiveHDHandshake{})
				communication.TrySendMotion(MotionDataPartial{})
				communication.TrySendONVIF(OnvifAction{})
			}
		}()
	}

	close(start)
	run.Shutdown(context.Background(), errors.New("test complete"))
	senders.Wait()
}

func TestCommunicationRunChannelLifecycleSoak(t *testing.T) {
	communication := &Communication{}
	for cycle := 0; cycle < 100; cycle++ {
		run := NewAgentRun(context.Background(), communication, false)
		if err := run.Activate(); err != nil {
			t.Fatalf("cycle %d Activate() error = %v", cycle, err)
		}
		handshakes := run.LiveHDHandshakes()
		motion := run.MotionEvents()
		onvif := run.ONVIFActions()

		var consumers sync.WaitGroup
		consumers.Add(3)
		go func() {
			defer consumers.Done()
			for range handshakes {
			}
		}()
		go func() {
			defer consumers.Done()
			for range motion {
			}
		}()
		go func() {
			defer consumers.Done()
			for range onvif {
			}
		}()

		var producers sync.WaitGroup
		for producer := 0; producer < 4; producer++ {
			producers.Add(1)
			go func() {
				defer producers.Done()
				for attempt := 0; attempt < 50; attempt++ {
					communication.TrySendLiveHDHandshake(LiveHDHandshake{})
					communication.TrySendMotion(MotionDataPartial{})
					communication.TrySendONVIF(OnvifAction{})
				}
			}()
		}

		run.Shutdown(context.Background(), errors.New("test complete"))
		producers.Wait()

		done := make(chan struct{})
		go func() {
			consumers.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("cycle %d consumers did not stop after channel close", cycle)
		}
	}
}

func TestCommunicationRecoveryTelemetry(t *testing.T) {
	communication := &Communication{}
	communication.RecordMoQReconnect(StreamQualityHigh)
	communication.RecordMoQReconnect(StreamQualityLow)
	communication.RecordMoQWrite(StreamQualityHigh, 1250*time.Millisecond, time.UnixMilli(1234))
	communication.RecordMoQWrite(StreamQualityLow, 2500*time.Millisecond, time.UnixMilli(5678))
	communication.RecordMoQWriteTimeout()
	communication.RecordWatchdogRestart(30 * time.Second)
	communication.RecordRunWorkerShutdownTimeout()

	if communication.TrySendLiveHDHandshake(LiveHDHandshake{}) {
		t.Fatal("TrySendLiveHDHandshake() accepted an unavailable channel")
	}
	if communication.TrySendMotion(MotionDataPartial{}) {
		t.Fatal("TrySendMotion() accepted an unavailable channel")
	}
	if communication.TrySendONVIF(OnvifAction{}) {
		t.Fatal("TrySendONVIF() accepted an unavailable channel")
	}

	got := communication.RecoveryTelemetry()
	if got.MoQHigh != (MoQRecoveryTelemetry{Reconnects: 1, LastFrameUnixMillis: 1234, LastWriteMillis: 1250}) {
		t.Fatalf("high MoQ telemetry = %+v", got.MoQHigh)
	}
	if got.MoQLow != (MoQRecoveryTelemetry{Reconnects: 1, LastFrameUnixMillis: 5678, LastWriteMillis: 2500}) {
		t.Fatalf("low MoQ telemetry = %+v", got.MoQLow)
	}
	if got.MoQWriteTimeouts != 1 || got.WatchdogRestarts != 1 || got.WatchdogCooldownSeconds != 30 || got.RunWorkerShutdownTimeouts != 1 {
		t.Fatalf("recovery telemetry = %+v", got)
	}
	if got.DroppedLiveHDHandshakes != 1 || got.DroppedMotionEvents != 1 || got.DroppedONVIFActions != 1 {
		t.Fatalf("drop telemetry = %+v", got)
	}
}

func TestCommunicationOperationalTelemetry(t *testing.T) {
	communication := &Communication{}
	mainPacketAt := time.Unix(1_788_710_401, 0)
	subPacketAt := time.Unix(1_788_710_402, 0)
	hubAttemptAt := time.Unix(1_788_710_403, 0)
	hubSuccessAt := time.Unix(1_788_710_404, 0)

	communication.SetStreamConfigured(MainStream, true)
	communication.SetStreamConfigured(SubStream, true)
	communication.RecordStreamPackage(MainStream, 29.97, 1920, 1080, mainPacketAt)
	communication.RecordStreamPackage(MainStream, 29.97, 1920, 1080, mainPacketAt)
	communication.RecordStreamPackage(SubStream, 15, 640, 360, subPacketAt)
	communication.SetHubConfigured(true)
	communication.RecordHubHeartbeatAttempt(hubAttemptAt)
	communication.RecordHubHeartbeatSuccess(hubSuccessAt)

	main := communication.StreamRuntimeTelemetry(MainStream)
	if main != (StreamRuntimeTelemetry{
		Configured:        true,
		PackagesProcessed: 2,
		FPS:               29.97,
		Width:             1920,
		Height:            1080,
		LastPacketAt:      mainPacketAt.Unix(),
	}) {
		t.Fatalf("main stream telemetry = %+v", main)
	}

	sub := communication.StreamRuntimeTelemetry(SubStream)
	if sub != (StreamRuntimeTelemetry{
		Configured:        true,
		PackagesProcessed: 1,
		FPS:               15,
		Width:             640,
		Height:            360,
		LastPacketAt:      subPacketAt.Unix(),
	}) {
		t.Fatalf("sub stream telemetry = %+v", sub)
	}

	hub := communication.HubRuntimeTelemetry()
	if hub != (HubRuntimeTelemetry{
		Configured:                true,
		Connected:                 true,
		LastHeartbeatAttemptAt:    hubAttemptAt.Unix(),
		LastSuccessfulHeartbeatAt: hubSuccessAt.Unix(),
	}) {
		t.Fatalf("Hub telemetry = %+v", hub)
	}

	communication.RecordHubHeartbeatFailure()
	if communication.HubRuntimeTelemetry().Connected {
		t.Fatal("Hub telemetry remained connected after a failed heartbeat")
	}
	communication.SetHubConfigured(false)
	if communication.HubRuntimeTelemetry().Configured {
		t.Fatal("Hub telemetry remained configured after it was disabled")
	}
}
