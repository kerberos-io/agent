package service

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kerberos-io/agent/examples/frame-processor/contract"
	"github.com/kerberos-io/agent/examples/frame-processor/processor"
)

type recordingPublish struct {
	deviceID string
	command  contract.RecordingWindowCommand
}

type fakePublisher struct {
	mu         sync.Mutex
	captures   []contract.CaptureFrameCommand
	recordings []recordingPublish
}

type blockingProcessor struct {
	started chan struct{}
	release chan struct{}
	mu      sync.Mutex
	calls   int
}

func (p *blockingProcessor) Process(ctx context.Context, _ contract.FrameMetadata, _ []byte) (processor.Decision, error) {
	p.mu.Lock()
	p.calls++
	if p.calls == 1 {
		close(p.started)
	}
	p.mu.Unlock()
	select {
	case <-ctx.Done():
		return processor.Decision{}, ctx.Err()
	case <-p.release:
		return processor.Decision{Triggered: true, Reason: "test"}, nil
	}
}

func (p *fakePublisher) PublishCaptureFrame(_ context.Context, _ string, command contract.CaptureFrameCommand) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.captures = append(p.captures, command)
	return nil
}

func (p *fakePublisher) PublishRecordingWindow(_ context.Context, deviceID string, command contract.RecordingWindowCommand) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.recordings = append(p.recordings, recordingPublish{deviceID: deviceID, command: command})
	return nil
}

func TestFrameAlwaysTriggerPublishesOneIdempotentRecordingCommand(t *testing.T) {
	publisher := &fakePublisher{}
	service := New(Config{
		APIToken: "secret", CommandTTL: time.Minute,
		PreRollSeconds: 10, EventClipSeconds: 30,
	}, processor.New(processor.Config{}), publisher, nil)
	service.now = func() time.Time { return time.UnixMilli(1_000) }
	server := httptest.NewServer(service.Handler())
	defer server.Close()

	metadata := validMetadata()
	for range 2 {
		response := postFrame(t, server.URL, "secret", metadata, jpegFrame(t, 2, 2, 255))
		if response.StatusCode != http.StatusOK {
			t.Fatalf("POST /v1/frames status = %d", response.StatusCode)
		}
		response.Body.Close()
	}

	if got := len(publisher.recordings); got != 1 {
		t.Fatalf("recording commands = %d, want 1", got)
	}
	published := publisher.recordings[0]
	if published.deviceID != metadata.DeviceID || published.command.CapturedAt != metadata.CapturedAt {
		t.Fatalf("published command = %#v", published)
	}
}

func TestFrameRequestPublishesCaptureCommand(t *testing.T) {
	publisher := &fakePublisher{}
	service := New(Config{APIToken: "secret"}, processor.New(processor.Config{}), publisher, func() string { return "generated-request" })
	service.now = func() time.Time { return time.UnixMilli(1_000) }
	server := httptest.NewServer(service.Handler())
	defer server.Close()

	body := `{"schemaVersion":"1.0","deviceIds":["device-1"],"processingProfile":"always-trigger","expiresAt":2000}`
	request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/frame-requests", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer secret")
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("POST /v1/frame-requests status = %d", response.StatusCode)
	}
	if got := len(publisher.captures); got != 1 || publisher.captures[0].RequestID != "generated-request" {
		t.Fatalf("capture commands = %#v", publisher.captures)
	}
}

func TestFrameRejectsUnauthorizedRequest(t *testing.T) {
	service := New(Config{APIToken: "secret"}, processor.New(processor.Config{}), &fakePublisher{}, nil)
	server := httptest.NewServer(service.Handler())
	defer server.Close()

	response := postFrame(t, server.URL, "wrong", validMetadata(), jpegFrame(t, 2, 2, 255))
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("POST /v1/frames status = %d", response.StatusCode)
	}
}

func TestFrameFailsClosedWithoutConfiguredToken(t *testing.T) {
	application := New(Config{}, processor.New(processor.Config{}), &fakePublisher{}, nil)
	server := httptest.NewServer(application.Handler())
	defer server.Close()

	response := postFrame(t, server.URL, "", validMetadata(), jpegFrame(t, 2, 2, 255))
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("POST /v1/frames status = %d", response.StatusCode)
	}
}

func TestConcurrentDuplicateFramesPublishOneRecordingCommand(t *testing.T) {
	publisher := &fakePublisher{}
	frameProcessor := &blockingProcessor{started: make(chan struct{}), release: make(chan struct{})}
	application := New(Config{
		APIToken: "secret", CommandTTL: time.Minute,
		PreRollSeconds: 10, EventClipSeconds: 30,
	}, frameProcessor, publisher, nil)
	application.now = func() time.Time { return time.UnixMilli(1_000) }
	server := httptest.NewServer(application.Handler())
	defer server.Close()

	metadata := validMetadata()
	statuses := make(chan int, 2)
	go func() {
		response := postFrame(t, server.URL, "secret", metadata, jpegFrame(t, 2, 2, 255))
		defer response.Body.Close()
		statuses <- response.StatusCode
	}()
	<-frameProcessor.started
	go func() {
		response := postFrame(t, server.URL, "secret", metadata, jpegFrame(t, 2, 2, 255))
		defer response.Body.Close()
		statuses <- response.StatusCode
	}()
	close(frameProcessor.release)

	for range 2 {
		if status := <-statuses; status != http.StatusOK {
			t.Fatalf("POST /v1/frames status = %d", status)
		}
	}
	if got := len(publisher.recordings); got != 1 {
		t.Fatalf("recording commands = %d, want 1", got)
	}
	frameProcessor.mu.Lock()
	defer frameProcessor.mu.Unlock()
	if frameProcessor.calls != 1 {
		t.Fatalf("processor calls = %d, want 1", frameProcessor.calls)
	}
}

func TestFrameRejectsTTLAboveConfiguredMaximum(t *testing.T) {
	application := New(Config{APIToken: "secret", MaxFrameTTL: time.Second}, processor.New(processor.Config{}), &fakePublisher{}, nil)
	application.now = func() time.Time { return time.UnixMilli(1_000) }
	server := httptest.NewServer(application.Handler())
	defer server.Close()

	metadata := validMetadata()
	metadata.ExpiresAt = 2_001
	response := postFrame(t, server.URL, "secret", metadata, jpegFrame(t, 2, 2, 255))
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("POST /v1/frames status = %d", response.StatusCode)
	}
}

func validMetadata() contract.FrameMetadata {
	return contract.FrameMetadata{
		SchemaVersion: contract.SchemaVersion,
		RequestID:     "request-1", FrameID: "frame-1", DeviceID: "device-1",
		CapturedAt: 900, ExpiresAt: 2_000, ProcessingProfile: processor.ProfileAlwaysTrigger,
		SourceStream: "sub", Width: 2, Height: 2,
	}
}

func jpegFrame(t *testing.T, width, height int, brightness uint8) []byte {
	t.Helper()
	frame := image.NewGray(image.Rect(0, 0, width, height))
	for index := range frame.Pix {
		frame.Pix[index] = brightness
	}
	frame.SetGray(0, 0, color.Gray{Y: brightness})
	var output bytes.Buffer
	if err := jpeg.Encode(&output, frame, nil); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func postFrame(t *testing.T, baseURL, token string, metadata contract.FrameMetadata, frame []byte) *http.Response {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	metadataHeader := make(textproto.MIMEHeader)
	metadataHeader.Set("Content-Disposition", `form-data; name="metadata"`)
	metadataHeader.Set("Content-Type", "application/json")
	part, err := writer.CreatePart(metadataHeader)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(part).Encode(metadata); err != nil {
		t.Fatal(err)
	}
	frameHeader := make(textproto.MIMEHeader)
	frameHeader.Set("Content-Disposition", `form-data; name="frame"; filename="frame.jpg"`)
	frameHeader.Set("Content-Type", "image/jpeg")
	part, err = writer.CreatePart(frameHeader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(frame); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request, err := http.NewRequest(http.MethodPost, baseURL+"/v1/frames", &body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
