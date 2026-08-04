package capture

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kerberos-io/agent/machinery/src/models"
)

func TestQueueRecordingForUploadSnapshotsFPS(t *testing.T) {
	configDirectory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(configDirectory, "data", "cloud"), 0o755); err != nil {
		t.Fatalf("mkdir cloud queue: %v", err)
	}

	configuration := &models.Configuration{}
	configuration.Config.Capture.IPCamera.FPS = "29.97"
	queueRecordingForUpload(configDirectory, "recording.mp4", configuration)

	got, err := os.ReadFile(filepath.Join(configDirectory, "data", "cloud", "recording.mp4"))
	if err != nil {
		t.Fatalf("read upload marker: %v", err)
	}
	if string(got) != "29.97" {
		t.Fatalf("upload marker FPS = %q, want %q", got, "29.97")
	}
}

func TestQueueRecordingForUploadKeepsUnknownFPSCompatible(t *testing.T) {
	for _, fps := range []string{"", "invalid", "0", "NaN", "241"} {
		t.Run(fps, func(t *testing.T) {
			configDirectory := t.TempDir()
			if err := os.MkdirAll(filepath.Join(configDirectory, "data", "cloud"), 0o755); err != nil {
				t.Fatalf("mkdir cloud queue: %v", err)
			}

			configuration := &models.Configuration{}
			configuration.Config.Capture.IPCamera.FPS = fps
			queueRecordingForUpload(configDirectory, "recording.mp4", configuration)

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
