package livemoq

import "time"

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

// Allow rejects stale frames and waits for a fresh keyframe before reopening.
func (g *FrameGate) Allow(isKeyFrame bool, capturedAtMs int64, now time.Time, maxAge time.Duration) (bool, FrameGateEvent) {
	if capturedAtMs > 0 && now.Sub(time.UnixMilli(capturedAtMs)) > maxAge {
		event := FrameGateEventNone
		if !g.recovering {
			event = FrameGateEventLagging
		}
		g.started = false
		g.recovering = true
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
