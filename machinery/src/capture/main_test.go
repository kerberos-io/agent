package capture

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/video"
)

func TestQueueRecordingForUploadStoresFinalizedMetadata(t *testing.T) {
	configDirectory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(configDirectory, "data", "cloud"), 0o755); err != nil {
		t.Fatalf("mkdir cloud queue: %v", err)
	}

	mp4Video := &video.MP4{VideoTotalDuration: 20452, SampleCount: 613}
	metadata := recordingUploadMetadata("recording.mp4", "device-key", 1785934709414, mp4Video)
	queueRecordingForUpload(configDirectory, metadata)

	got, err := os.ReadFile(filepath.Join(configDirectory, "data", "cloud", "recording.metadata"))
	if err != nil {
		t.Fatalf("read upload marker: %v", err)
	}
	var stored models.RecordingUploadMetadata
	if err := json.Unmarshal(got, &stored); err != nil {
		t.Fatalf("decode upload marker: %v", err)
	}
	expectedFPS := mp4Video.AverageFPS()
	if stored.FileName != "recording.mp4" || stored.DeviceKey != "device-key" || stored.Timestamp != 1785934709414 || stored.Duration != 20452 || math.Abs(stored.FPS-expectedFPS) > 1e-9 {
		t.Fatalf("upload marker = %+v", stored)
	}
	if stored.FPS == math.Floor(stored.FPS) {
		t.Fatalf("upload marker FPS = %v, want fractional precision", stored.FPS)
	}
}

func TestQueueRecordingForUploadKeepsUnknownFPSCompatible(t *testing.T) {
	for _, fps := range []float64{0, 0.99, -1, math.NaN(), math.Inf(1), 241} {
		t.Run("invalid FPS", func(t *testing.T) {
			configDirectory := t.TempDir()
			if err := os.MkdirAll(filepath.Join(configDirectory, "data", "cloud"), 0o755); err != nil {
				t.Fatalf("mkdir cloud queue: %v", err)
			}

			metadata := models.RecordingUploadMetadata{FileName: "recording.mp4"}
			if fps >= 1 && fps <= 240 && !math.IsNaN(fps) && !math.IsInf(fps, 0) {
				metadata.FPS = fps
			}
			queueRecordingForUpload(configDirectory, metadata)

			got, err := os.ReadFile(filepath.Join(configDirectory, "data", "cloud", "recording.metadata"))
			if err != nil {
				t.Fatalf("read upload marker: %v", err)
			}
			if string(got) != `{"filename":"recording.mp4","device_key":"","timestamp":0,"duration":0}` {
				t.Fatalf("upload marker = %q, want metadata without FPS", got)
			}
		})
	}
}
