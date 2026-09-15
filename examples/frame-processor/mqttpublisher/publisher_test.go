package mqttpublisher

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kerberos-io/agent/examples/frame-processor/contract"
)

func TestNewMessageMatchesAgentEnvelope(t *testing.T) {
	command := contract.CaptureFrameCommand{
		SchemaVersion:     contract.SchemaVersion,
		RequestID:         "request-1",
		ProcessingProfile: "always-trigger",
		ExpiresAt:         2_000,
	}
	message := newMessage("device-1", contract.ActionCaptureFrame, command, time.Unix(1_000, 0))
	payload, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["device_id"] != "device-1" || decoded["timestamp"] != float64(1_000) {
		t.Fatalf("envelope = %s", payload)
	}
	inner := decoded["payload"].(map[string]any)
	if inner["action"] != contract.ActionCaptureFrame || inner["device_id"] != "device-1" {
		t.Fatalf("payload = %#v", inner)
	}
}

func TestStatusMessageHandlerIgnoresOtherActions(t *testing.T) {
	called := false
	handler := statusMessageHandler(func(contract.StatusEvent) { called = true })
	handler(nil, fakeMessage(`{"payload":{"action":"motion","value":{}}}`))
	if called {
		t.Fatal("handler accepted an unrelated action")
	}
}

type fakeMessage string

func (m fakeMessage) Duplicate() bool   { return false }
func (m fakeMessage) Qos() byte         { return 1 }
func (m fakeMessage) Retained() bool    { return false }
func (m fakeMessage) Topic() string     { return "test" }
func (m fakeMessage) MessageID() uint16 { return 1 }
func (m fakeMessage) Payload() []byte   { return []byte(m) }
func (m fakeMessage) Ack()              {}
