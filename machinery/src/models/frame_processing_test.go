package models

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestFrameProcessingTokenIsNotSerialized(t *testing.T) {
	config := Config{FrameProcessing: &FrameProcessing{
		Enabled:  "true",
		Endpoint: "https://processor.example/v1/frames",
		Token:    "do-not-expose",
	}}
	value, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(value), config.FrameProcessing.Token) {
		t.Fatalf("serialized config exposed frame-processing token: %s", value)
	}
}

func TestFrameProcessingRuntimeTelemetry(t *testing.T) {
	communication := &Communication{}
	communication.SetFrameProcessingConfigured(true)
	communication.RecordFrameProcessingSample()
	communication.RecordFrameProcessingQueued(1, false)
	communication.RecordFrameProcessingQueued(1, true)
	communication.SetFrameProcessingQueueDepth(0)
	communication.RecordFrameProcessingFailure()
	communication.RecordFrameProcessingSuccess(time.Unix(123, 0))

	got := communication.FrameProcessingRuntimeTelemetry()
	if !got.Configured || got.Sampled != 1 || got.Queued != 2 || got.Dropped != 1 || got.Failed != 1 || got.Submitted != 1 {
		t.Fatalf("FrameProcessingRuntimeTelemetry() = %+v", got)
	}
	if got.QueueDepth != 0 || got.LastSuccessAt != 123 {
		t.Fatalf("FrameProcessingRuntimeTelemetry() timing = %+v", got)
	}
}
