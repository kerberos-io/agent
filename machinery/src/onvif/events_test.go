package onvif

import (
	"context"
	"testing"
	"time"

	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/onvif/event/stream"
	"github.com/stretchr/testify/assert"
)

func makeConfig(recording, onvifMotion, name string) *models.Configuration {
	return &models.Configuration{
		Name: name,
		Config: models.Config{
			Capture: models.Capture{
				Recording:   recording,
				ONVIFMotion: onvifMotion,
			},
		},
	}
}

func makeCommunication(buffer int) *models.Communication {
	return &models.Communication{
		HandleMotion: make(chan models.MotionDataPartial, buffer),
	}
}

// --- dispatchEvent ---------------------------------------------------

func TestDispatchEvent_MotionActive_SendsToHandleMotion(t *testing.T) {
	cfg := makeConfig("true", "true", "cam-1")
	comm := makeCommunication(1)
	ev := stream.Event{Kind: stream.KindMotion, State: stream.StateActive}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dispatchEvent(ctx, ev, cfg, comm)

	select {
	case m := <-comm.HandleMotion:
		assert.NotZero(t, m.Timestamp)
	case <-time.After(time.Second):
		t.Fatal("expected motion data on HandleMotion")
	}
}

func TestDispatchEvent_MotionInactive_DoesNotSend(t *testing.T) {
	cfg := makeConfig("true", "true", "cam-1")
	comm := makeCommunication(1)
	ev := stream.Event{Kind: stream.KindMotion, State: stream.StateInactive}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dispatchEvent(ctx, ev, cfg, comm)

	select {
	case <-comm.HandleMotion:
		t.Fatal("inactive motion must not reach HandleMotion (motion-stop is a follow-up)")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestDispatchEvent_NonMotionKindIgnored(t *testing.T) {
	cfg := makeConfig("true", "true", "cam-1")
	comm := makeCommunication(1)
	ev := stream.Event{Kind: stream.KindDigitalInput, State: stream.StateActive}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dispatchEvent(ctx, ev, cfg, comm)

	select {
	case <-comm.HandleMotion:
		t.Fatal("non-motion kinds must not reach HandleMotion")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestDispatchEvent_RecordingDisabled_DoesNotSend(t *testing.T) {
	cfg := makeConfig("false", "true", "cam-1")
	comm := makeCommunication(1)
	ev := stream.Event{Kind: stream.KindMotion, State: stream.StateActive}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dispatchEvent(ctx, ev, cfg, comm)

	select {
	case <-comm.HandleMotion:
		t.Fatal("Recording=false must gate the send (matches computervision behaviour)")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestDispatchEvent_HandleMotionFull_DropsRatherThanBlocks(t *testing.T) {
	cfg := makeConfig("true", "true", "cam-1")
	// Pre-fill the buffer so the next send would block.
	comm := &models.Communication{HandleMotion: make(chan models.MotionDataPartial, 1)}
	comm.HandleMotion <- models.MotionDataPartial{}
	ev := stream.Event{Kind: stream.KindMotion, State: stream.StateActive}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		dispatchEvent(ctx, ev, cfg, comm)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("dispatchEvent must drop when HandleMotion is full, not block")
	}
}

func TestDispatchEvent_CtxCancelledAndHandleMotionClosed_DoesNotPanic(t *testing.T) {
	// Regression for the shutdown race: between cancel() and
	// close(HandleMotion) the agent leaves a 3s window. If dispatchEvent
	// runs in that window AFTER the channel is closed, a non-protected
	// send would panic. The ctx pre-check must short-circuit before the
	// send is attempted.
	cfg := makeConfig("true", "true", "cam-1")
	comm := &models.Communication{HandleMotion: make(chan models.MotionDataPartial, 1)}
	close(comm.HandleMotion)
	ev := stream.Event{Kind: stream.KindMotion, State: stream.StateActive}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled, matching the shutdown sequence

	assert.NotPanics(t, func() {
		dispatchEvent(ctx, ev, cfg, comm)
	})
}

// --- isONVIFMotionEnabled --------------------------------------------

func TestIsONVIFMotionEnabled_CaseAndWhitespace(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"true", true},
		{"True", true},
		{"TRUE", true},
		{" true", true},
		{"true ", true},
		{"  true  ", true},
		{"false", false},
		{"False", false},
		{"", false},
		{"yes", false},
		{"1", false},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, isONVIFMotionEnabled(tc.in))
		})
	}
}

// --- resolveDeviceID -------------------------------------------------

func TestResolveDeviceID_FallbackChain(t *testing.T) {
	tests := []struct {
		name    string
		cfgName string
		xaddr   string
		want    string
	}{
		{"name_set", "front-door", "192.168.1.10", "front-door"},
		{"name_empty_xaddr_set", "", "192.168.1.10", "192.168.1.10"},
		{"name_whitespace_only_xaddr_set", "  ", "192.168.1.10", "192.168.1.10"},
		{"both_empty", "", "", "unknown"},
		{"name_with_trailing_whitespace", "cam-2  ", "192.168.1.10", "cam-2"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, resolveDeviceID(tc.cfgName, tc.xaddr))
		})
	}
}
