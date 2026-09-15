package models

import "github.com/kerberos-io/agent/machinery/src/packets"

const (
	FrameProcessingSchemaVersion = "1.0"
	FrameProcessingStatusAction  = "frame-processing-status"
)

type FrameProcessingRequest struct {
	SchemaVersion     string `json:"schemaVersion"`
	RequestID         string `json:"requestId"`
	ProcessingProfile string `json:"processingProfile"`
	ExpiresAt         int64  `json:"expiresAt"`
	TraceID           string `json:"traceId,omitempty"`
}

type FrameProcessingWork struct {
	Request FrameProcessingRequest
	Cursor  *packets.QueueCursor
}

type FrameProcessingStatus struct {
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
