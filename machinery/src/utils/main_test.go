package utils

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func TestAgentEnvironmentVariableNamesOmitsValues(t *testing.T) {
	got := agentEnvironmentVariableNames([]string{
		"AGENT_HUB_PRIVATE_KEY=do-not-log",
		"PATH=/usr/bin",
		"NOT_AGENT_SECRET=also-do-not-log",
		"AGENT_CAPTURE_LIVEVIEW=true",
	})
	want := []string{"AGENT_CAPTURE_LIVEVIEW", "AGENT_HUB_PRIVATE_KEY"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("agentEnvironmentVariableNames() = %v, want %v", got, want)
	}
}

func TestConfigurationLogFieldsOmitCredentialsAndEndpoints(t *testing.T) {
	config := models.Config{
		Name:          "Front Door",
		FriendlyName:  "Entrance",
		Cloud:         "kstorage",
		HubKey:        "hub-key-secret",
		HubPrivateKey: "hub-private-secret",
		MQTTURI:       "mqtt://internal.example",
		MQTTUsername:  "mqtt-user",
		MQTTPassword:  "mqtt-password-secret",
		Capture: models.Capture{
			Liveview:           "true",
			MaxLengthRecording: 20,
			IPCamera: models.IPCamera{
				RTSP:          "rtsp://camera-user:camera-password@10.0.30.11/live",
				ONVIFUsername: "onvif-user",
				ONVIFPassword: "onvif-password-secret",
			},
		},
	}

	fields := []map[string]interface{}{
		configurationLogFields(config),
		configurationDebugLogFields(config),
	}
	summary := fmt.Sprint(fields)
	for _, secret := range []string{
		config.HubKey,
		config.HubPrivateKey,
		config.MQTTURI,
		config.MQTTUsername,
		config.MQTTPassword,
		config.Capture.IPCamera.RTSP,
		config.Capture.IPCamera.ONVIFUsername,
		config.Capture.IPCamera.ONVIFPassword,
	} {
		if strings.Contains(summary, secret) {
			t.Fatalf("configuration log fields exposed sensitive value %q in %q", secret, summary)
		}
	}
	infoFields := configurationLogFields(config)
	if got := infoFields["agent_name"]; got != "Front Door" {
		t.Fatalf("agent_name = %v, want Front Door", got)
	}
	if got := infoFields["cloud_provider"]; got != "kstorage" {
		t.Fatalf("cloud_provider = %v, want kstorage", got)
	}
	debugFields := configurationDebugLogFields(config)
	if got := debugFields["max_recording_seconds"]; got != int64(20) {
		t.Fatalf("max_recording_seconds = %v, want 20", got)
	}
	if got := debugFields["main_stream_configured"]; got != true {
		t.Fatalf("main_stream_configured = %v, want true", got)
	}
}

type stubFileInfo struct {
	name    string
	modTime time.Time
}

func (s stubFileInfo) Name() string       { return s.name }
func (s stubFileInfo) Size() int64        { return 0 }
func (s stubFileInfo) Mode() os.FileMode  { return 0 }
func (s stubFileInfo) ModTime() time.Time { return s.modTime }
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

func TestGetMediaFormattedSupportsOpaqueFileNames(t *testing.T) {
	root := t.TempDir()
	recordingDirectory := filepath.Join(root, "recordings")
	cloudDirectory := filepath.Join(root, "cloud")
	if err := os.MkdirAll(cloudDirectory, 0755); err != nil {
		t.Fatal(err)
	}
	fileName := "opaque recording.mp4"
	marker := []byte(`{"filename":"opaque recording.mp4","timestamp":1785934709414}`)
	if err := os.WriteFile(filepath.Join(cloudDirectory, models.RecordingUploadMetadataFileName(fileName)), marker, 0644); err != nil {
		t.Fatal(err)
	}
	configuration := &models.Configuration{}
	configuration.Config.Timezone = "UTC"

	media := GetMediaFormatted([]os.FileInfo{stubFileInfo{name: fileName}}, recordingDirectory, configuration, models.EventFilter{})
	if len(media) != 1 || media[0].Timestamp != "1785934709" {
		t.Fatalf("GetMediaFormatted() = %#v", media)
	}
}

func TestRecordingTimestampFallsBackToModificationTime(t *testing.T) {
	want := time.Unix(1785934709, 0)
	timestamp, ok := recordingTimestamp(stubFileInfo{name: "opaque.mp4", modTime: want}, "/tmp/recordings")
	if !ok || timestamp != want.Unix() {
		t.Fatalf("recordingTimestamp() = %d/%v, want %d/true", timestamp, ok, want.Unix())
	}
}
