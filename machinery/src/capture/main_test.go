package capture

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestQueueRecordingForUploadStoresFinalizedFPS(t *testing.T) {
	configDirectory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(configDirectory, "data", "cloud"), 0o755); err != nil {
		t.Fatalf("mkdir cloud queue: %v", err)
	}

	queueRecordingForUpload(configDirectory, "recording.mp4", 29.970029)

	got, err := os.ReadFile(filepath.Join(configDirectory, "data", "cloud", "recording.mp4"))
	if err != nil {
		t.Fatalf("read upload marker: %v", err)
	}
	if string(got) != "29.97" {
		t.Fatalf("upload marker FPS = %q, want %q", got, "29.97")
	}
}

func TestQueueRecordingForUploadKeepsUnknownFPSCompatible(t *testing.T) {
	for _, fps := range []float64{0, -1, math.NaN(), math.Inf(1), 241} {
		t.Run("invalid FPS", func(t *testing.T) {
			configDirectory := t.TempDir()
			if err := os.MkdirAll(filepath.Join(configDirectory, "data", "cloud"), 0o755); err != nil {
				t.Fatalf("mkdir cloud queue: %v", err)
			}

			queueRecordingForUpload(configDirectory, "recording.mp4", fps)

			got, err := os.ReadFile(filepath.Join(configDirectory, "data", "cloud", "recording.mp4"))
			if err != nil {
				t.Fatalf("read upload marker: %v", err)
			}
			if len(got) != 0 {
				t.Fatalf("upload marker = %q, want empty", got)
			}
		})
	}
}
