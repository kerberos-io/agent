package utils

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"testing"
	"time"

	"github.com/kerberos-io/agent/machinery/src/models"
)

func TestImageToBytesReturnsCompleteJPEG(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 16, 12))
	source.Set(8, 6, color.RGBA{R: 255, A: 255})
	var input image.Image = source

	encoded, err := ImageToBytes(&input)
	if err != nil {
		t.Fatalf("ImageToBytes() error = %v", err)
	}
	if len(encoded) == 0 {
		t.Fatal("ImageToBytes() returned an empty JPEG")
	}

	decoded, err := jpeg.Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("decoding ImageToBytes() output: %v", err)
	}
	if got := decoded.Bounds().Size(); got.X != 16 || got.Y != 12 {
		t.Fatalf("decoded JPEG size = %dx%d, want 16x12", got.X, got.Y)
	}
}

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
