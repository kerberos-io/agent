package cloud

import (
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const recordingFPSHeader = "X-Kerberos-Storage-Fps"

// queuedRecordingFPS reads the FPS snapshot written into the upload marker
// when the recording was finalized. Historical empty markers intentionally
// return no value so receivers can retain their existing MP4-derived fallback.
func queuedRecordingFPS(fileName string) string {
	value, err := os.ReadFile(filepath.Join("data", "cloud", filepath.Base(fileName)))
	if err != nil {
		return ""
	}

	fps := strings.TrimSpace(string(value))
	parsed, err := strconv.ParseFloat(fps, 64)
	if err != nil || parsed <= 0 || parsed > 240 || math.IsInf(parsed, 0) || math.IsNaN(parsed) {
		return ""
	}
	return fps
}

func setQueuedRecordingFPSHeader(header http.Header, fileName string) {
	if fps := queuedRecordingFPS(fileName); fps != "" {
		header.Set(recordingFPSHeader, fps)
	}
}
