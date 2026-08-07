package livemoq

import (
	"testing"
	"time"
)

func TestFrameGateRecoversAtFreshKeyframe(t *testing.T) {
	now := time.UnixMilli(10_000)
	maxAge := 1500 * time.Millisecond
	gate := FrameGate{}

	tests := []struct {
		name         string
		isKeyFrame   bool
		capturedAtMs int64
		wantAllowed  bool
		wantEvent    FrameGateEvent
	}{
		{name: "waits for initial keyframe", capturedAtMs: 10_000},
		{name: "starts at initial keyframe", isKeyFrame: true, capturedAtMs: 10_000, wantAllowed: true, wantEvent: FrameGateEventStarted},
		{name: "publishes fresh delta", capturedAtMs: 10_020, wantAllowed: true},
		{name: "detects stale packet", capturedAtMs: 8_000, wantEvent: FrameGateEventLagging},
		{name: "rejects fresh delta while recovering", capturedAtMs: 10_040},
		{name: "rejects stale keyframe without duplicate event", isKeyFrame: true, capturedAtMs: 8_000},
		{name: "recovers at fresh keyframe", isKeyFrame: true, capturedAtMs: 10_060, wantAllowed: true, wantEvent: FrameGateEventRecovered},
		{name: "publishes delta after recovery", capturedAtMs: 10_080, wantAllowed: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			allowed, event := gate.Allow(test.isKeyFrame, test.capturedAtMs, now, maxAge)
			if allowed != test.wantAllowed {
				t.Fatalf("Allow() allowed = %t, want %t", allowed, test.wantAllowed)
			}
			if event != test.wantEvent {
				t.Fatalf("Allow() event = %d, want %d", event, test.wantEvent)
			}
		})
	}
}

func TestFrameGateAllowsMissingCaptureTime(t *testing.T) {
	gate := FrameGate{}
	allowed, event := gate.Allow(true, 0, time.Now(), time.Second)
	if !allowed || event != FrameGateEventStarted {
		t.Fatalf("Allow() = (%t, %d), want (true, %d)", allowed, event, FrameGateEventStarted)
	}
}
