package utils

import (
	"os"
	"testing"
	"time"

	"github.com/kerberos-io/agent/machinery/src/models"
)

type stubFileInfo struct {
	name string
}

func (s stubFileInfo) Name() string       { return s.name }
func (s stubFileInfo) Size() int64        { return 0 }
func (s stubFileInfo) Mode() os.FileMode  { return 0 }
func (s stubFileInfo) ModTime() time.Time { return time.Unix(0, 0) }
func (s stubFileInfo) IsDir() bool        { return false }
func (s stubFileInfo) Sys() interface{}   { return nil }

func TestGetMediaFormattedHonorsTimestampRange(t *testing.T) {
	configuration := &models.Configuration{}
	configuration.Config.Timezone = "UTC"
	configuration.Config.Name = "Front Door"
	configuration.Config.Key = "camera-1"

	files := []os.FileInfo{
		stubFileInfo{name: "1700000200_6_7_8_9_10.mp4"},
		stubFileInfo{name: "1700000100_6_7_8_9_10.mp4"},
		stubFileInfo{name: "1700000000_6_7_8_9_10.mp4"},
	}

	media := GetMediaFormatted(files, "/tmp/recordings", configuration, models.EventFilter{
		TimestampOffsetStart: 1700000050,
		TimestampOffsetEnd:   1700000200,
		NumberOfElements:     10,
	})

	if len(media) != 1 {
		t.Fatalf("expected 1 media item in time range, got %d", len(media))
	}

	if media[0].Timestamp != "1700000100" {
		t.Fatalf("expected timestamp 1700000100, got %s", media[0].Timestamp)
	}

	if media[0].CameraName != "Front Door" {
		t.Fatalf("expected camera name to be preserved, got %s", media[0].CameraName)
	}
	if media[0].CameraKey != "camera-1" {
		t.Fatalf("expected camera key to be preserved, got %s", media[0].CameraKey)
	}
}
