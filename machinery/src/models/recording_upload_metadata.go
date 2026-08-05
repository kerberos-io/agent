package models

import (
	"path/filepath"
	"strings"
)

const RecordingUploadMetadataExtension = ".metadata"

// RecordingUploadMetadata is persisted in the upload queue marker associated
// with a recording. New optional fields can be added without changing the queue
// mechanism or breaking older agents.
type RecordingUploadMetadata struct {
	FPS int `json:"fps,omitempty"`
}

// RecordingUploadMetadataFileName returns the queue marker name associated
// with a recording, replacing the recording extension with .metadata.
func RecordingUploadMetadataFileName(recordingFileName string) string {
	name := filepath.Base(recordingFileName)
	extension := filepath.Ext(name)
	return strings.TrimSuffix(name, extension) + RecordingUploadMetadataExtension
}

// RecordingFileNameFromUploadMarker resolves a queue entry to its recording.
// Markers created by older agents used the recording filename directly.
func RecordingFileNameFromUploadMarker(markerFileName string) string {
	name := filepath.Base(markerFileName)
	if strings.HasSuffix(name, RecordingUploadMetadataExtension) {
		return strings.TrimSuffix(name, RecordingUploadMetadataExtension) + ".mp4"
	}
	return name
}
