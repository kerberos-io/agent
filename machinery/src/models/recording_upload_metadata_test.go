package models

import "testing"

func TestRecordingUploadMetadataFileNames(t *testing.T) {
	if got := RecordingUploadMetadataFileName("141245_x_x_.mp4"); got != "141245_x_x_.metadata" {
		t.Fatalf("RecordingUploadMetadataFileName() = %q", got)
	}
	if got := RecordingFileNameFromUploadMarker("141245_x_x_.metadata"); got != "141245_x_x_.mp4" {
		t.Fatalf("RecordingFileNameFromUploadMarker() = %q", got)
	}
	if got := RecordingFileNameFromUploadMarker("legacy.mp4"); got != "legacy.mp4" {
		t.Fatalf("legacy RecordingFileNameFromUploadMarker() = %q", got)
	}
}
