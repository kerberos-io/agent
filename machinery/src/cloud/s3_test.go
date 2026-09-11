package cloud

import (
	"testing"

	"github.com/kerberos-io/agent/machinery/src/models"
)

func TestS3ObjectMetadataUsesStructuredRecordingMetadata(t *testing.T) {
	metadata := s3ObjectMetadata(models.RecordingUploadMetadata{
		Timestamp:         1785934709414,
		Duration:          20452,
		DeviceName:        "camera-name",
		RegionCoordinates: "1-2-3-4",
		NumberOfChanges:   "57",
	}, "device-key", "public-key")

	for key, want := range map[string]string{
		"event-timestamp":         "1785934709",
		"event-microseconds":      "414",
		"event-instancename":      "camera-name",
		"event-regioncoordinates": "1-2-3-4",
		"event-numberofchanges":   "57",
		"event-duration":          "20452",
		"event-token":             "20452",
		"productid":               "device-key",
		"publickey":               "public-key",
	} {
		if metadata[key] != want {
			t.Errorf("metadata[%q] = %q, want %q", key, metadata[key], want)
		}
	}
}

func TestLegacyS3RecordingMetadata(t *testing.T) {
	metadata, ok := legacyS3RecordingMetadata("1785934709_3-414_camera_1-2-3-4_57_20452.mp4", "device-key")
	if !ok || metadata.Timestamp != 1785934709414 || metadata.Duration != 20452 || metadata.DeviceName != "camera" || metadata.RegionCoordinates != "1-2-3-4" || metadata.NumberOfChanges != "57" {
		t.Fatalf("legacyS3RecordingMetadata() = %#v/%v", metadata, ok)
	}
}
