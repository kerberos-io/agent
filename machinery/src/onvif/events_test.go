package onvif

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/onvif/event/stream"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// captureDebugLog redirects logrus to a buffer at debug level for the
// duration of a test and returns what was written. It mutates package
// globals, so callers must not run in parallel.
func captureDebugLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prevOut, prevLevel := logrus.StandardLogger().Out, logrus.GetLevel()
	logrus.SetOutput(&buf)
	logrus.SetLevel(logrus.DebugLevel)
	t.Cleanup(func() {
		logrus.SetOutput(prevOut)
		logrus.SetLevel(prevLevel)
	})
	return &buf
}

// TestDispatchEvent_LogsTheTriggeringTopic — a dispatched event is what
// actually starts a recording, so its topic is the one an operator needs
// when a camera records for the wrong reason (or the right reason and
// nobody can prove which). Rejected events were already logged; without
// this the triggering topic is only knowable by elimination.
func TestDispatchEvent_LogsTheTriggeringTopic(t *testing.T) {
	buf := captureDebugLog(t)

	cfg := makeConfig("true", "true", "cam-1")
	comm := makeCommunication(1)
	ev := stream.Event{
		Kind:  stream.KindMotion,
		State: stream.StateActive,
		Topic: "tns1:RuleEngine/tnsaxis:VMD3/vmd3_video_1",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dispatchEvent(ctx, ev, cfg, comm)

	assert.Contains(t, buf.String(), "tns1:RuleEngine/tnsaxis:VMD3/vmd3_video_1",
		"the dispatched event's topic must appear in the log")
	assert.Contains(t, buf.String(), "Motion",
		"the dispatched event's Kind must appear in the log")
}

// TestDispatchEvent_PropertyOperation — a camera replays the current
// state of every property topic as Initialized whenever a pull-point
// subscription is created. If that counts as a trigger, every
// reconnect restarts a recording for any motion property that happens
// to be active, and a flapping subscription manufactures motion out of
// nothing. Only reject Initialized specifically: PropertyOperation is
// optional per WS-Notification and absent on many non-property events,
// which decode reports as PropertyUnknown.
func TestDispatchEvent_PropertyOperation(t *testing.T) {
	tests := []struct {
		name     string
		op       stream.PropertyOperation
		wantSend bool
	}{
		{"changed is a real transition", stream.PropertyChanged, true},
		{"absent attribute still counts", stream.PropertyUnknown, true},
		{"initialized is a subscription state replay", stream.PropertyInitialized, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := makeConfig("true", "true", "cam-1")
			comm := makeCommunication(1)
			ev := stream.Event{
				Kind:      stream.KindMotion,
				State:     stream.StateActive,
				Operation: tt.op,
				Topic:     "tns1:RuleEngine/tnsaxis:VMD3/vmd3_video_1",
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			dispatchEvent(ctx, ev, cfg, comm)

			select {
			case <-comm.HandleMotion:
				if !tt.wantSend {
					t.Fatalf("%v must not trigger a recording", tt.op)
				}
			case <-time.After(100 * time.Millisecond):
				if tt.wantSend {
					t.Fatalf("%v must trigger a recording", tt.op)
				}
			}
		})
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

// TestDispatchEvent_OnlyRealTransitionsTrigger — a camera replays every
// property topic's state on each new subscription (Initialized) and
// announces removals (Deleted). Neither is a motion transition, and a
// flapping pull-point would otherwise manufacture recordings out of
// replayed state. PropertyOperation is optional per WS-Notification, so
// absent (Unknown) still counts — many non-property events omit it.
func TestDispatchEvent_OnlyRealTransitionsTrigger(t *testing.T) {
	tests := []struct {
		op       stream.PropertyOperation
		wantSend bool
	}{
		{stream.PropertyChanged, true},
		{stream.PropertyUnknown, true},
		{stream.PropertyInitialized, false},
		{stream.PropertyDeleted, false},
	}

	for _, tt := range tests {
		t.Run(tt.op.String(), func(t *testing.T) {
			cfg := makeConfig("true", "true", "cam-1")
			comm := makeCommunication(1)
			ev := stream.Event{Kind: stream.KindMotion, State: stream.StateActive, Operation: tt.op}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			dispatchEvent(ctx, ev, cfg, comm)

			if tt.wantSend {
				require.Len(t, comm.HandleMotion, 1, "%v must trigger a recording", tt.op)
				return
			}
			require.Empty(t, comm.HandleMotion, "%v must not trigger a recording", tt.op)
		})
	}
}

// TestSanitiseTopic — ev.Topic is camera-controlled and reaches the log
// unmodified. logrus's coloured text formatter (the default) writes the
// message without quoting, so an embedded newline forges whole log
// lines: a compromised camera can fabricate ERROR entries or spoof
// another device's id, in the logs an operator is reading to diagnose
// that very camera. Length is also unbounded on the wire, and the
// reject path logs every event, so an oversized topic is a cheap way to
// evict a container's whole retained history.
func TestSanitiseTopic(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"ordinary topic passes through", "tns1:RuleEngine/tnsaxis:VMD3/vmd3_video_1", "tns1:RuleEngine/tnsaxis:VMD3/vmd3_video_1"},
		{"newline cannot forge a line", "a\nERRO[fake] boom", `a\nERRO[fake] boom`},
		{"carriage return", "a\rb", `a\rb`},
		{"tab", "a\tb", `a\tb`},
		{"NUL", "a\x00b", `a\x00b`},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitiseTopic(tt.in)
			assert.Equal(t, tt.want, got)
			assert.NotContains(t, got, "\n", "no raw newline may survive")
			assert.NotContains(t, got, "\r", "no raw carriage return may survive")
		})
	}
}

func TestSanitiseTopic_Truncates(t *testing.T) {
	got := sanitiseTopic(strings.Repeat("x", maxLoggedTopic*2))
	assert.LessOrEqual(t, len(got), maxLoggedTopic+len("…(truncated)"))
	assert.Contains(t, got, "truncated")
}

// TestDispatchEvent_LogsTriggerOnlyWhenSent — the trigger line is the
// record that a recording started. Logging it before the send means a
// dropped event (full channel, or shutdown) leaves a line claiming a
// recording that never began.
func TestDispatchEvent_LogsTriggerOnlyWhenSent(t *testing.T) {
	buf := captureDebugLog(t)

	cfg := makeConfig("true", "true", "cam-1")
	comm := &models.Communication{HandleMotion: make(chan models.MotionDataPartial, 1)}
	comm.HandleMotion <- models.MotionDataPartial{} // full
	ev := stream.Event{Kind: stream.KindMotion, State: stream.StateActive, Topic: "tns1:VideoSource/MotionAlarm"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dispatchEvent(ctx, ev, cfg, comm)

	assert.NotContains(t, buf.String(), "recording trigger",
		"a dropped event must not be logged as a trigger")
	assert.Contains(t, buf.String(), "dropping", "the drop itself must still be logged")
}
