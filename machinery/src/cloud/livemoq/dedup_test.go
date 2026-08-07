package livemoq

import (
	"testing"
	"time"
)

func TestKeyframeDeduplicator(t *testing.T) {
	now := time.UnixMilli(10_000)
	window := 500 * time.Millisecond
	payload := []byte{0x00, 0x00, 0x00, 0x01, 0x65, 0x88}
	deduplicator := KeyframeDeduplicator{}

	if deduplicator.IsDuplicate(1_000, 10_000, payload, now, window) {
		t.Fatal("first keyframe reported as duplicate")
	}
	if !deduplicator.IsDuplicate(1_000, 10_020, payload, now.Add(20*time.Millisecond), window) {
		t.Fatal("exact repeated keyframe was not reported as duplicate")
	}
	if deduplicator.IsDuplicate(2_000, 11_000, payload, now.Add(time.Second), window) {
		t.Fatal("same payload with a new timestamp reported as duplicate")
	}
	if deduplicator.IsDuplicate(2_000, 11_020, append(payload, 0x01), now.Add(1020*time.Millisecond), window) {
		t.Fatal("different payload with the same timestamp reported as duplicate")
	}
}

func TestKeyframeDeduplicatorAllowsTimestampReuseOutsideWindow(t *testing.T) {
	now := time.UnixMilli(10_000)
	payload := []byte{0x00, 0x00, 0x00, 0x01, 0x65, 0x88}
	deduplicator := KeyframeDeduplicator{}

	deduplicator.IsDuplicate(1_000, 10_000, payload, now, 500*time.Millisecond)
	if deduplicator.IsDuplicate(1_000, 20_000, payload, now.Add(10*time.Second), 500*time.Millisecond) {
		t.Fatal("later keyframe after timestamp reset reported as duplicate")
	}
}

func TestKeyframeDeduplicatorFallsBackToObservationTime(t *testing.T) {
	now := time.UnixMilli(10_000)
	payload := []byte{0x65, 0x88}
	deduplicator := KeyframeDeduplicator{}

	deduplicator.IsDuplicate(1_000, 0, payload, now, 500*time.Millisecond)
	if !deduplicator.IsDuplicate(1_000, 0, payload, now.Add(20*time.Millisecond), 500*time.Millisecond) {
		t.Fatal("duplicate without capture time was not reported")
	}
}

func TestKeyframeDeduplicatorReset(t *testing.T) {
	now := time.UnixMilli(10_000)
	payload := []byte{0x65, 0x88}
	deduplicator := KeyframeDeduplicator{}

	deduplicator.IsDuplicate(1_000, 10_000, payload, now, 500*time.Millisecond)
	deduplicator.Reset()
	if deduplicator.IsDuplicate(1_000, 10_020, payload, now.Add(20*time.Millisecond), 500*time.Millisecond) {
		t.Fatal("first keyframe after reset reported as duplicate")
	}
}
