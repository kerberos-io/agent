package cloud

import (
	"encoding/json"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kerberos-io/agent/machinery/src/models"
)

const recordingFPSHeader = "X-Kerberos-Storage-Fps"
const recordingDurationHeader = "X-Kerberos-Storage-Duration"
const recordingTimestampHeader = "X-Kerberos-Storage-Timestamp"

// queuedRecordingFPS reads the FPS snapshot written into the upload marker
// when the recording was finalized. Historical empty markers intentionally
// return no value so receivers can retain their existing MP4-derived fallback.
func queuedRecordingFPS(fileName string) string {
	value, ok := readRecordingUploadMetadata(fileName)
	if !ok {
		return ""
	}

	marker := strings.TrimSpace(string(value))
	if strings.HasPrefix(marker, "{") {
		metadata, ok := decodeRecordingUploadMetadata(value)
		if !ok || metadata.FPS <= 0 || metadata.FPS > 240 {
			return ""
		}
		return strconv.Itoa(metadata.FPS)
	}

	// Compatibility with markers created before upload metadata used JSON.
	fps := marker
	parsed, err := strconv.ParseFloat(fps, 64)
	if err != nil || parsed <= 0 || parsed > 240 || math.IsInf(parsed, 0) || math.IsNaN(parsed) {
		return ""
	}
	return fps
}

func queuedRecordingMetadata(fileName string) (models.RecordingUploadMetadata, bool) {
	value, ok := readRecordingUploadMetadata(fileName)
	if !ok || !strings.HasPrefix(strings.TrimSpace(string(value)), "{") {
		return models.RecordingUploadMetadata{}, false
	}
	return decodeRecordingUploadMetadata(value)
}

func decodeRecordingUploadMetadata(value []byte) (models.RecordingUploadMetadata, bool) {
	var metadata models.RecordingUploadMetadata
	if err := json.Unmarshal(value, &metadata); err != nil {
		return models.RecordingUploadMetadata{}, false
	}
	return metadata, true
}

func readRecordingUploadMetadata(fileName string) ([]byte, bool) {
	markerNames := []string{
		models.RecordingUploadMetadataFileName(fileName),
		filepath.Base(fileName),
	}
	for _, markerName := range markerNames {
		value, err := os.ReadFile(filepath.Join("data", "cloud", markerName))
		if err == nil {
			return value, true
		}
	}
	return nil, false
}

func setQueuedRecordingMetadataHeaders(header http.Header, fileName string) {
	if fps := queuedRecordingFPS(fileName); fps != "" {
		header.Set(recordingFPSHeader, fps)
	}
	if metadata, ok := queuedRecordingMetadata(fileName); ok {
		if metadata.Duration > 0 {
			header.Set(recordingDurationHeader, strconv.FormatUint(metadata.Duration, 10))
		}
		if metadata.Timestamp > 0 {
			header.Set(recordingTimestampHeader, strconv.FormatInt(metadata.Timestamp, 10))
		}
	}
}
