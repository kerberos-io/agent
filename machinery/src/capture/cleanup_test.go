package capture

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/kerberos-io/agent/machinery/src/models"
)

// writeRecording creates a file under recordingsDir and sets its modtime so the
// tests can control the "oldest" ordering deterministically.
func writeRecording(t *testing.T, recordingsDir, name string, ageMinutes int) {
	t.Helper()
	full := filepath.Join(recordingsDir, name)
	if err := os.WriteFile(full, []byte("data"), 0o644); err != nil {
		t.Fatalf("write recording %s: %v", name, err)
	}
	mod := time.Now().Add(-time.Duration(ageMinutes) * time.Minute)
	if err := os.Chtimes(full, mod, mod); err != nil {
		t.Fatalf("chtimes %s: %v", name, err)
	}
}

// markPending creates the upload marker in cloudDir for the given recording,
// marking it as still queued for upload.
func markPending(t *testing.T, cloudDir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(cloudDir, name), nil, 0o644); err != nil {
		t.Fatalf("write marker %s: %v", name, err)
	}
}

func newCleanupDirs(t *testing.T) (string, string) {
	t.Helper()
	base := t.TempDir()
	recordingsDir := filepath.Join(base, "data", "recordings")
	cloudDir := filepath.Join(base, "data", "cloud")
	if err := os.MkdirAll(recordingsDir, 0o755); err != nil {
		t.Fatalf("mkdir recordings: %v", err)
	}
	if err := os.MkdirAll(cloudDir, 0o755); err != nil {
		t.Fatalf("mkdir cloud: %v", err)
	}
	return recordingsDir, cloudDir
}

// The core regression: when the oldest recording is still pending upload but a
// newer one has already been uploaded, cleanup must delete the uploaded (safe)
// one and leave the pending recording on disk so it can still be uploaded.
func TestPickRecordingToCleanup_PrefersUploaded(t *testing.T) {
	recordingsDir, cloudDir := newCleanupDirs(t)

	// oldest is still pending upload (marker present).
	writeRecording(t, recordingsDir, "oldest_pending.mp4", 30)
	markPending(t, cloudDir, "oldest_pending.mp4")
	// newer one has already been uploaded (no marker).
	writeRecording(t, recordingsDir, "newer_uploaded.mp4", 10)

	name, pending, err := pickRecordingToCleanup(recordingsDir, cloudDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pending {
		t.Fatalf("expected a safe (already-uploaded) deletion, got pending=true")
	}
	if name != "newer_uploaded.mp4" {
		t.Fatalf("cleanup picked %q, want the uploaded recording newer_uploaded.mp4", name)
	}
}

// Among several already-uploaded recordings, the oldest uploaded one is chosen.
func TestPickRecordingToCleanup_OldestUploadedFirst(t *testing.T) {
	recordingsDir, cloudDir := newCleanupDirs(t)

	writeRecording(t, recordingsDir, "old_uploaded.mp4", 40)
	writeRecording(t, recordingsDir, "mid_uploaded.mp4", 20)
	// pending one must be ignored even though it is not the oldest.
	writeRecording(t, recordingsDir, "pending.mp4", 30)
	markPending(t, cloudDir, "pending.mp4")

	name, pending, err := pickRecordingToCleanup(recordingsDir, cloudDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pending {
		t.Fatalf("expected pending=false, got true")
	}
	if name != "old_uploaded.mp4" {
		t.Fatalf("cleanup picked %q, want old_uploaded.mp4", name)
	}
}

// Last resort: when every recording is still pending upload, cleanup returns the
// oldest one with pending=true so the caller can drop it (and its marker) to keep
// the disk bounded.
func TestPickRecordingToCleanup_AllPendingFallsBackToOldest(t *testing.T) {
	recordingsDir, cloudDir := newCleanupDirs(t)

	writeRecording(t, recordingsDir, "a_old.mp4", 50)
	markPending(t, cloudDir, "a_old.mp4")
	writeRecording(t, recordingsDir, "b_new.mp4", 5)
	markPending(t, cloudDir, "b_new.mp4")

	name, pending, err := pickRecordingToCleanup(recordingsDir, cloudDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !pending {
		t.Fatalf("expected pending=true when every recording is queued for upload")
	}
	if name != "a_old.mp4" {
		t.Fatalf("cleanup picked %q, want the oldest pending a_old.mp4", name)
	}
}

// An empty recordings directory yields os.ErrNotExist so the caller does nothing.
func TestPickRecordingToCleanup_Empty(t *testing.T) {
	recordingsDir, cloudDir := newCleanupDirs(t)

	if _, _, err := pickRecordingToCleanup(recordingsDir, cloudDir); err != os.ErrNotExist {
		t.Fatalf("expected os.ErrNotExist for an empty directory, got %v", err)
	}
}

// writeSizedRecording writes a recording of an exact byte size so tests can
// exercise the megabyte-based directory-cap threshold.
func writeSizedRecording(t *testing.T, dir, name string, size int) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), make([]byte, size), 0o644); err != nil {
		t.Fatalf("write sized recording %s: %v", name, err)
	}
}

// When AGENT_AUTO_CLEAN_MAX_SIZE (MaxDirectorySize) is set, cleanup triggers once
// the recordings directory grows past that many megabytes.
func TestRecordingsNeedCleanup_FixedCap(t *testing.T) {
	recordingsDir, _ := newCleanupDirs(t)
	// ~2 MB of recordings on disk.
	writeSizedRecording(t, recordingsDir, "big.mp4", 2*1000*1000)

	over := &models.Configuration{Config: models.Config{MaxDirectorySize: 1}}
	need, err := recordingsNeedCleanup(recordingsDir, over)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !need {
		t.Fatalf("expected cleanup when 2MB of recordings exceed the 1MB cap")
	}

	under := &models.Configuration{Config: models.Config{MaxDirectorySize: 100}}
	need, err = recordingsNeedCleanup(recordingsDir, under)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if need {
		t.Fatalf("expected no cleanup when 2MB of recordings stay under the 100MB cap")
	}
}

// With no fixed cap (the default), cleanup is driven by the free space left on
// the recordings filesystem versus the reserve.
func TestRecordingsNeedCleanup_DefaultDiskReserve(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("disk usage stats are only implemented on linux")
	}
	recordingsDir, _ := newCleanupDirs(t)

	totalMB, availableMB, err := diskUsageMB(recordingsDir)
	if err != nil {
		t.Fatalf("diskUsageMB: %v", err)
	}
	if totalMB <= 0 || availableMB <= 0 {
		t.Skipf("unexpected disk stats total=%dMB available=%dMB", totalMB, availableMB)
	}

	// A reserve larger than the whole disk means free space is always below it.
	over := &models.Configuration{Config: models.Config{MinFreeSpace: totalMB + availableMB}}
	need, err := recordingsNeedCleanup(recordingsDir, over)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !need {
		t.Fatalf("expected cleanup when free space (%dMB) is below the reserve", availableMB)
	}

	// A 1 MB reserve leaves plenty of free space, so nothing should be cleaned.
	under := &models.Configuration{Config: models.Config{MinFreeSpace: 1}}
	need, err = recordingsNeedCleanup(recordingsDir, under)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if need {
		t.Fatalf("expected no cleanup when free space (%dMB) exceeds the 1MB reserve", availableMB)
	}
}
