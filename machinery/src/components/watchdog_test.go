package components

import (
	"context"
	"testing"
	"time"
)

func TestWaitForRunRetryStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	started := time.Now()
	if waitForRunRetry(ctx) {
		t.Fatal("waitForRunRetry() completed the retry delay after cancellation")
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("waitForRunRetry() took %s after cancellation", elapsed)
	}
}

func TestStreamRestartWatchdogCoalescesStallsAndBacksOff(t *testing.T) {
	now := time.Unix(1_000, 0)
	watchdog := newStreamRestartWatchdog()
	if _, restart := watchdog.Observe(now, 1, 1, true, false); restart {
		t.Fatal("initial observation requested a restart")
	}

	for check := 1; check < streamWatchdogStallChecks; check++ {
		now = now.Add(streamWatchdogInterval)
		if _, restart := watchdog.Observe(now, 1, 1, true, false); restart {
			t.Fatalf("check %d requested an early restart", check)
		}
	}
	now = now.Add(streamWatchdogInterval)
	reason, restart := watchdog.Observe(now, 1, 1, true, false)
	if !restart || reason != "main and sub streams" {
		t.Fatalf("Observe() = (%q, %t), want coalesced restart", reason, restart)
	}

	watchdog.MarkRestart(now)
	if got := watchdog.Backoff(); got != 30*time.Second {
		t.Fatalf("Backoff() = %s, want 30s", got)
	}
	for now = now.Add(streamWatchdogInterval); now.Before(watchdog.nextRestart); now = now.Add(streamWatchdogInterval) {
		if _, restart := watchdog.Observe(now, 1, 1, true, false); restart {
			t.Fatal("restart requested during cooldown")
		}
	}
}

func TestStreamRestartWatchdogResetsAfterHealthyMinute(t *testing.T) {
	now := time.Unix(2_000, 0)
	watchdog := newStreamRestartWatchdog()
	watchdog.backoff = streamWatchdogMaxBackoff
	watchdog.nextRestart = now.Add(streamWatchdogMaxBackoff)
	watchdog.Observe(now, 1, 1, true, false)

	for elapsed := streamWatchdogInterval; elapsed <= streamWatchdogHealthyReset+streamWatchdogInterval; elapsed += streamWatchdogInterval {
		now = now.Add(streamWatchdogInterval)
		watchdog.Observe(now, int64(elapsed), int64(elapsed), true, false)
	}

	if got := watchdog.Backoff(); got != streamWatchdogBaseBackoff {
		t.Fatalf("Backoff() = %s, want %s", got, streamWatchdogBaseBackoff)
	}
	if !watchdog.nextRestart.IsZero() {
		t.Fatalf("nextRestart = %s, want zero", watchdog.nextRestart)
	}
}

func TestStreamRestartWatchdogPausesWhileConfiguring(t *testing.T) {
	now := time.Unix(3_000, 0)
	watchdog := newStreamRestartWatchdog()
	watchdog.Observe(now, 1, 0, false, false)
	for check := 0; check < streamWatchdogStallChecks+1; check++ {
		now = now.Add(streamWatchdogInterval)
		if _, restart := watchdog.Observe(now, 1, 0, false, true); restart {
			t.Fatal("restart requested while configuring")
		}
	}
}
