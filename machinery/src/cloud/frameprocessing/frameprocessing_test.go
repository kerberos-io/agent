package frameprocessing

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/packets"
)

type fakeDecoder struct{}

func (fakeDecoder) DecodePacket(packets.Packet) (image.YCbCr, error) {
	frame := image.NewYCbCr(image.Rect(0, 0, 4, 4), image.YCbCrSubsampleRatio420)
	for index := range frame.Y {
		frame.Y[index] = color.Gray{Y: 200}.Y
	}
	return *frame, nil
}

type fakeStatusPublisher struct {
	statuses chan models.FrameProcessingStatus
}

func (p *fakeStatusPublisher) Publish(_ context.Context, status models.FrameProcessingStatus) error {
	p.statuses <- status
	return nil
}

func TestEnqueueLatestReplacesOldestFrame(t *testing.T) {
	frames := make(chan Frame, 1)
	frames <- Frame{Metadata: Metadata{FrameID: "old"}}

	if dropped := enqueueLatest(frames, Frame{Metadata: Metadata{FrameID: "new"}}); !dropped {
		t.Fatal("enqueueLatest() did not report dropping the stale frame")
	}
	if got := (<-frames).Metadata.FrameID; got != "new" {
		t.Fatalf("queued frame = %q, want new", got)
	}
}

func TestPrepareFrameUsesAgentCaptureTimestamp(t *testing.T) {
	now := time.UnixMilli(2_000)
	frame, err := prepareFrame(packets.Packet{CurrentTime: 1_500}, fakeDecoder{}, models.FrameProcessing{
		Profile: "never-trigger", Width: 2, Height: 2, JPEGQuality: 70, FrameTTLSeconds: 30, MaxFrameBytes: 4 << 20,
	}, "device-1", "sub", now)
	if err != nil {
		t.Fatal(err)
	}
	if frame.Metadata.CapturedAt != 1_500 || frame.Metadata.ExpiresAt != 32_000 {
		t.Fatalf("metadata timestamps = %+v", frame.Metadata)
	}
	if frame.Metadata.Width != 2 || frame.Metadata.Height != 2 || len(frame.JPEG) == 0 {
		t.Fatalf("prepared frame = %+v, bytes=%d", frame.Metadata, len(frame.JPEG))
	}
}

func TestSenderSubmitsContractMultipartRequest(t *testing.T) {
	var gotMetadata Metadata
	var gotFrame []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q", got)
		}
		reader, err := request.MultipartReader()
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Error(err)
				return
			}
			switch part.FormName() {
			case "metadata":
				if err := json.NewDecoder(part).Decode(&gotMetadata); err != nil {
					t.Error(err)
				}
			case "frame":
				gotFrame, err = io.ReadAll(part)
				if err != nil {
					t.Error(err)
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"decision":"no-event"}`))
	}))
	defer server.Close()

	sender, err := NewSender(models.FrameProcessing{Endpoint: server.URL, Token: "secret", RequestTimeoutSeconds: 2})
	if err != nil {
		t.Fatal(err)
	}
	want := Frame{Metadata: Metadata{SchemaVersion: schemaVersion, FrameID: "frame-1"}, JPEG: []byte("jpeg")}
	if err := sender.Submit(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	if gotMetadata.FrameID != want.Metadata.FrameID || !bytes.Equal(gotFrame, want.JPEG) {
		t.Fatalf("submitted metadata=%+v frame=%q", gotMetadata, gotFrame)
	}
}

func TestNewSenderRejectsRelativeEndpoint(t *testing.T) {
	if _, err := NewSender(models.FrameProcessing{Endpoint: "/v1/frames", Token: "secret", RequestTimeoutSeconds: 1}); err == nil {
		t.Fatal("NewSender() accepted a relative endpoint")
	}
}

func TestValidateConfigRejectsUnboundedQueue(t *testing.T) {
	config := models.FrameProcessing{
		Profile: "never-trigger", IntervalSeconds: 10, Width: 640,
		JPEGQuality: 70, RequestTimeoutSeconds: 5, FrameTTLSeconds: 30,
		MaxFrameBytes: 4 << 20, PeriodicQueueCapacity: 65,
	}
	if err := validateConfig(config); err == nil {
		t.Fatal("validateConfig() accepted an unbounded queue")
	}
}

func TestMultipartContentTypeIsParseable(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if _, err := request.MultipartReader(); err != nil {
		t.Fatal(err)
	}
}

func TestRunCancelsBlockedPacketRead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	queue := packets.NewQueue()
	defer queue.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, queue.Latest(), fakeDecoder{}, models.FrameProcessing{
			Enabled: "true", Endpoint: server.URL, Token: "secret", Profile: "never-trigger",
			Stream: "main", IntervalSeconds: 10, Width: 640, JPEGQuality: 70,
			RequestTimeoutSeconds: 2, FrameTTLSeconds: 30, MaxFrameBytes: 4 << 20,
			PeriodicQueueCapacity: 1,
		}, "device-1", "main", nil)
	}()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not stop after cancellation")
	}
}

func TestRunRequestedCapturesNextKeyframeAndSubmitsHTTP(t *testing.T) {
	metadataReceived := make(chan Metadata, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		reader, err := request.MultipartReader()
		if err != nil {
			t.Error(err)
			return
		}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Error(err)
				return
			}
			if part.FormName() == "metadata" {
				var metadata Metadata
				if err := json.NewDecoder(part).Decode(&metadata); err != nil {
					t.Error(err)
					return
				}
				metadataReceived <- metadata
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	queue := packets.NewQueue()
	defer queue.Close()
	if err := queue.WriteHeader([]packets.Stream{{Index: 0, IsVideo: true}}); err != nil {
		t.Fatal(err)
	}
	requests := make(chan models.FrameProcessingWork, 1)
	statuses := &fakeStatusPublisher{statuses: make(chan models.FrameProcessingStatus, 4)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	config := models.FrameProcessing{
		Enabled: "true", Endpoint: server.URL, Token: "secret", Profile: "never-trigger",
		IntervalSeconds: 10, Width: 2, Height: 2, JPEGQuality: 70,
		RequestTimeoutSeconds: 2, FrameTTLSeconds: 30, MaxFrameBytes: 4 << 20,
		PeriodicQueueCapacity: 1,
	}
	go func() {
		done <- RunRequested(ctx, fakeDecoder{}, config, "device-1", "sub", requests, statuses, nil)
	}()
	requests <- models.FrameProcessingWork{
		Request: models.FrameProcessingRequest{
			SchemaVersion: models.FrameProcessingSchemaVersion,
			RequestID:     "request-1", ProcessingProfile: "always-trigger",
			ExpiresAt: time.Now().Add(time.Second).UnixMilli(), TraceID: "trace-1",
		},
		Cursor: queue.LatestAtCurrentTail(),
	}
	queue.WritePacket(packets.Packet{Idx: 0, IsVideo: true, IsKeyFrame: true, CurrentTime: 1234, Data: []byte{1}})

	metadata := <-metadataReceived
	if metadata.RequestID != "request-1" || metadata.CapturedAt != 1234 || metadata.TraceID != "trace-1" {
		t.Fatalf("submitted metadata = %+v", metadata)
	}
	status := <-statuses.statuses
	if status.Status != "submitted" || status.FrameID == "" {
		t.Fatalf("status = %+v", status)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("RunRequested() error = %v", err)
	}
}

func TestRunRequestedExpiresWhileWaitingForKeyframe(t *testing.T) {
	queue := packets.NewQueue()
	defer queue.Close()
	requests := make(chan models.FrameProcessingWork, 1)
	statuses := &fakeStatusPublisher{statuses: make(chan models.FrameProcessingStatus, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- RunRequested(ctx, fakeDecoder{}, models.FrameProcessing{
			Enabled: "true", Endpoint: "http://127.0.0.1:1/v1/frames", Token: "secret", Profile: "never-trigger",
			IntervalSeconds: 10, Width: 2, Height: 2, JPEGQuality: 70,
			RequestTimeoutSeconds: 1, FrameTTLSeconds: 30, MaxFrameBytes: 4 << 20,
			PeriodicQueueCapacity: 1,
		}, "device-1", "main", requests, statuses, nil)
	}()
	requests <- models.FrameProcessingWork{
		Request: models.FrameProcessingRequest{
			SchemaVersion: models.FrameProcessingSchemaVersion,
			RequestID:     "request-expiring", ProcessingProfile: "never-trigger",
			ExpiresAt: time.Now().Add(20 * time.Millisecond).UnixMilli(),
		},
		Cursor: queue.LatestAtCurrentTail(),
	}
	select {
	case status := <-statuses.statuses:
		if status.Status != "expired" {
			t.Fatalf("status = %+v", status)
		}
	case <-time.After(time.Second):
		t.Fatal("requested frame did not expire while waiting for a keyframe")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("RunRequested() error = %v", err)
	}
}
