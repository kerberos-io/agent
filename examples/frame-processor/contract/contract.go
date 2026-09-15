package contract

import (
	"errors"
	"fmt"
)

const (
	SchemaVersion     = "1.0"
	MaxImageDimension = 8192

	ActionCaptureFrame           = "capture-frame"
	ActionRequestRecordingWindow = "request-recording-window"
	ActionFrameStatus            = "frame-processing-status"
)

type FrameMetadata struct {
	SchemaVersion     string `json:"schemaVersion"`
	RequestID         string `json:"requestId"`
	FrameID           string `json:"frameId"`
	DeviceID          string `json:"deviceId"`
	CapturedAt        int64  `json:"capturedAt"`
	ExpiresAt         int64  `json:"expiresAt"`
	ProcessingProfile string `json:"processingProfile"`
	SourceStream      string `json:"sourceStream"`
	Width             int    `json:"width"`
	Height            int    `json:"height"`
	TraceID           string `json:"traceId,omitempty"`
}

func (m FrameMetadata) Validate(nowMillis int64) error {
	if m.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schemaVersion %q", m.SchemaVersion)
	}
	if m.RequestID == "" || m.FrameID == "" || m.DeviceID == "" {
		return errors.New("requestId, frameId, and deviceId are required")
	}
	if m.CapturedAt <= 0 {
		return errors.New("capturedAt must be a positive Unix millisecond timestamp")
	}
	if m.ExpiresAt <= m.CapturedAt {
		return errors.New("expiresAt must be later than capturedAt")
	}
	if nowMillis > 0 && m.ExpiresAt <= nowMillis {
		return errors.New("frame has expired")
	}
	if m.ProcessingProfile == "" {
		return errors.New("processingProfile is required")
	}
	if m.SourceStream != "main" && m.SourceStream != "sub" {
		return errors.New("sourceStream must be main or sub")
	}
	if m.Width <= 0 || m.Height <= 0 || m.Width > MaxImageDimension || m.Height > MaxImageDimension {
		return fmt.Errorf("width and height must be between 1 and %d", MaxImageDimension)
	}
	return nil
}

type FrameRequest struct {
	SchemaVersion     string   `json:"schemaVersion"`
	RequestID         string   `json:"requestId,omitempty"`
	DeviceIDs         []string `json:"deviceIds"`
	ProcessingProfile string   `json:"processingProfile"`
	ExpiresAt         int64    `json:"expiresAt"`
	TraceID           string   `json:"traceId,omitempty"`
}

func (r FrameRequest) Validate(nowMillis int64) error {
	if r.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schemaVersion %q", r.SchemaVersion)
	}
	if len(r.DeviceIDs) == 0 {
		return errors.New("at least one deviceId is required")
	}
	for _, deviceID := range r.DeviceIDs {
		if deviceID == "" {
			return errors.New("deviceIds cannot contain empty values")
		}
	}
	if r.ProcessingProfile == "" {
		return errors.New("processingProfile is required")
	}
	if r.ExpiresAt <= nowMillis {
		return errors.New("expiresAt must be in the future")
	}
	return nil
}

type CaptureFrameCommand struct {
	SchemaVersion     string `json:"schemaVersion"`
	RequestID         string `json:"requestId"`
	ProcessingProfile string `json:"processingProfile"`
	ExpiresAt         int64  `json:"expiresAt"`
	TraceID           string `json:"traceId,omitempty"`
}

type RecordingWindowCommand struct {
	SchemaVersion     string `json:"schemaVersion"`
	RequestID         string `json:"requestId"`
	FrameID           string `json:"frameId"`
	CapturedAt        int64  `json:"capturedAt"`
	PreRollSeconds    int64  `json:"preRollSeconds"`
	EventClipSeconds  int64  `json:"eventClipSeconds"`
	ExpiresAt         int64  `json:"expiresAt"`
	ProcessingProfile string `json:"processingProfile"`
	TraceID           string `json:"traceId,omitempty"`
}

func (c RecordingWindowCommand) Validate(nowMillis int64) error {
	if c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schemaVersion %q", c.SchemaVersion)
	}
	if c.RequestID == "" || c.FrameID == "" {
		return errors.New("requestId and frameId are required")
	}
	if c.CapturedAt <= 0 {
		return errors.New("capturedAt must be a positive Unix millisecond timestamp")
	}
	if c.EventClipSeconds <= 0 {
		return errors.New("eventClipSeconds must be positive")
	}
	if c.PreRollSeconds < 0 || c.PreRollSeconds > c.EventClipSeconds {
		return errors.New("preRollSeconds must be between zero and eventClipSeconds")
	}
	if c.ExpiresAt <= nowMillis {
		return errors.New("expiresAt must be in the future")
	}
	return nil
}

type StatusEvent struct {
	SchemaVersion string `json:"schemaVersion"`
	RequestID     string `json:"requestId"`
	FrameID       string `json:"frameId,omitempty"`
	DeviceID      string `json:"deviceId"`
	Status        string `json:"status"`
	OccurredAt    int64  `json:"occurredAt"`
	Retryable     bool   `json:"retryable,omitempty"`
	Message       string `json:"message,omitempty"`
	TraceID       string `json:"traceId,omitempty"`
}

type MQTTMessage struct {
	MID         string      `json:"mid"`
	DeviceID    string      `json:"device_id"`
	Timestamp   int64       `json:"timestamp"`
	Encrypted   bool        `json:"encrypted"`
	Hidden      bool        `json:"hidden"`
	PublicKey   string      `json:"public_key"`
	Fingerprint string      `json:"fingerprint"`
	Payload     MQTTPayload `json:"payload"`
}

type MQTTPayload struct {
	Version        string `json:"version"`
	Action         string `json:"action"`
	DeviceID       string `json:"device_id"`
	Signature      string `json:"signature"`
	EncryptedValue string `json:"encrypted_value"`
	HiddenValue    string `json:"hidden_value"`
	Value          any    `json:"value"`
}
