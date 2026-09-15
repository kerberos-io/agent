package contract

import (
	"encoding/json"
	"os"
	"testing"
)

func TestFrameMetadataValidate(t *testing.T) {
	now := int64(1_000)
	metadata := FrameMetadata{
		SchemaVersion:     SchemaVersion,
		RequestID:         "request-1",
		FrameID:           "frame-1",
		DeviceID:          "device-1",
		CapturedAt:        900,
		ExpiresAt:         1_100,
		ProcessingProfile: "always-trigger",
		SourceStream:      "sub",
		Width:             640,
		Height:            480,
	}

	if err := metadata.Validate(now); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	metadata.ExpiresAt = now
	if err := metadata.Validate(now); err == nil {
		t.Fatal("Validate() accepted an expired frame")
	}
}

func TestRecordingWindowCommandValidate(t *testing.T) {
	command := RecordingWindowCommand{
		SchemaVersion:    SchemaVersion,
		RequestID:        "request-1",
		FrameID:          "frame-1",
		CapturedAt:       900,
		PreRollSeconds:   10,
		EventClipSeconds: 30,
		ExpiresAt:        2_000,
	}

	if err := command.Validate(1_000); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	command.PreRollSeconds = 31
	if err := command.Validate(1_000); err == nil {
		t.Fatal("Validate() accepted pre-roll longer than the event clip")
	}
}

func TestContractFixturesDecode(t *testing.T) {
	tests := []struct {
		path   string
		target any
	}{
		{"../testdata/frame-request.json", &FrameRequest{}},
		{"../testdata/capture-frame.json", &CaptureFrameCommand{}},
		{"../testdata/request-recording-window.json", &RecordingWindowCommand{}},
	}
	for _, test := range tests {
		value, err := os.ReadFile(test.path)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(value, test.target); err != nil {
			t.Fatalf("decode %s: %v", test.path, err)
		}
	}
}
