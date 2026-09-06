package livemoq

import (
	"sync/atomic"
	"time"
)

type FrameGateEvent uint8

const (
	FrameGateEventNone FrameGateEvent = iota
	FrameGateEventStarted
	FrameGateEventLagging
	FrameGateEventRecovered
)

// FrameGate keeps publication on a decodable, recent GOP.
type FrameGate struct {
	started    bool
	recovering bool
}

// AudienceGate publishes every frame while watched and only keyframes while
// idle. The idle keyframes keep the relay's latest cached group current without
// paying the bandwidth cost of a full stream when nobody is watching.
type AudienceGate struct {
	idle bool
}

func (g *AudienceGate) Allow(hasSubscribers bool, isKeyFrame bool) (allowed bool, enteredIdle bool) {
	if hasSubscribers {
		g.idle = false
		return true, false
	}

	enteredIdle = !g.idle
	g.idle = true
	return isKeyFrame, enteredIdle
}

// WriteWatchdog tracks the single synchronous frame write performed by a MoQ
// publisher so another goroutine can interrupt a wedged native call.
type WriteWatchdog struct {
	startedAt atomic.Int64
}

func (w *WriteWatchdog) Begin(now time.Time) {
	w.startedAt.Store(now.UnixNano())
}

func (w *WriteWatchdog) End() {
	w.startedAt.Store(0)
}

func (w *WriteWatchdog) Elapsed(now time.Time) (time.Duration, bool) {
	startedAt := w.startedAt.Load()
	if startedAt == 0 {
		return 0, false
	}
	elapsed := now.Sub(time.Unix(0, startedAt))
	if elapsed < 0 {
		elapsed = 0
	}
	return elapsed, true
}

// Reset closes the gate so publication resumes on the next keyframe. Entering
// idle mode uses it to start the relay-cache refresh on a complete GOP.
func (g *FrameGate) Reset() {
	g.started = false
	g.recovering = false
}

// Allow rejects stale frames and waits for a fresh keyframe before reopening.
func (g *FrameGate) Allow(isKeyFrame bool, capturedAtMs int64, now time.Time, maxAge time.Duration) (bool, FrameGateEvent) {
	if capturedAtMs > 0 && now.Sub(time.UnixMilli(capturedAtMs)) > maxAge {
		event := FrameGateEventNone
		if g.started {
			if !g.recovering {
				event = FrameGateEventLagging
			}
			g.started = false
			g.recovering = true
		}
		return false, event
	}

	if !g.started {
		if !isKeyFrame {
			return false, FrameGateEventNone
		}
		g.started = true
		if g.recovering {
			g.recovering = false
			return true, FrameGateEventRecovered
		}
		return true, FrameGateEventStarted
	}

	return true, FrameGateEventNone
}
