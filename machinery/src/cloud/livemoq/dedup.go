package livemoq

import (
	"crypto/sha256"
	"time"
)

type KeyframeDeduplicator struct {
	hasPrevious  bool
	timestampMs  int64
	capturedAtMs int64
	observedAt   time.Time
	digest       [sha256.Size]byte
}

func (d *KeyframeDeduplicator) Reset() {
	*d = KeyframeDeduplicator{}
}

// IsDuplicate reports exact repeated keyframe access units observed close
// together. Distinct IDR slices within one access unit remain untouched.
func (d *KeyframeDeduplicator) IsDuplicate(timestampMs int64, capturedAtMs int64, payload []byte, observedAt time.Time, window time.Duration) bool {
	digest := sha256.Sum256(payload)
	duplicate := d.hasPrevious && d.timestampMs == timestampMs && d.digest == digest
	if duplicate {
		if capturedAtMs > 0 && d.capturedAtMs > 0 {
			gap := time.Duration(capturedAtMs-d.capturedAtMs) * time.Millisecond
			duplicate = gap >= 0 && gap <= window
		} else {
			gap := observedAt.Sub(d.observedAt)
			duplicate = gap >= 0 && gap <= window
		}
	}

	d.hasPrevious = true
	d.timestampMs = timestampMs
	d.capturedAtMs = capturedAtMs
	d.observedAt = observedAt
	d.digest = digest
	return duplicate
}
