package frameprocessing

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gofrs/uuid"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/packets"
	"github.com/kerberos-io/agent/machinery/src/utils"
	log "github.com/sirupsen/logrus"
)

const (
	schemaVersion        = "1.0"
	maxResponseBodyBytes = 64 << 10
)

type Decoder interface {
	DecodePacket(packets.Packet) (image.YCbCr, error)
}

type Observer interface {
	SetFrameProcessingConfigured(bool)
	RecordFrameProcessingSample()
	RecordFrameProcessingQueued(int, bool)
	SetFrameProcessingQueueDepth(int)
	RecordFrameProcessingSuccess(time.Time)
	RecordFrameProcessingFailure()
}

type Metadata struct {
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

type Frame struct {
	Metadata Metadata
	JPEG     []byte
}

type Sender struct {
	endpoint string
	token    string
	client   *http.Client
}

type StatusPublisher interface {
	Publish(context.Context, models.FrameProcessingStatus) error
}

type MQTTStatusPublisher struct {
	client        mqtt.Client
	hubKey        string
	configuration *models.Configuration
	timeout       time.Duration
}

func NewMQTTStatusPublisher(client mqtt.Client, hubKey string, configuration *models.Configuration) *MQTTStatusPublisher {
	return &MQTTStatusPublisher{
		client: client, hubKey: hubKey, configuration: configuration, timeout: 5 * time.Second,
	}
}

func (p *MQTTStatusPublisher) Publish(ctx context.Context, status models.FrameProcessingStatus) error {
	if p == nil || p.client == nil || p.hubKey == "" || p.configuration == nil {
		return errors.New("frame-processing MQTT status publisher is not configured")
	}
	value, err := structToMap(status)
	if err != nil {
		return err
	}
	payload, err := models.PackageMQTTMessage(p.configuration, models.Message{
		Payload: models.Payload{
			Version:  schemaVersion,
			Action:   models.FrameProcessingStatusAction,
			DeviceId: status.DeviceID,
			Value:    value,
		},
	})
	if err != nil {
		return fmt.Errorf("package frame-processing status: %w", err)
	}
	token := p.client.Publish("kerberos/hub/"+p.hubKey, 1, false, payload)
	timer := time.NewTimer(p.timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return errors.New("frame-processing status publish timed out")
	case <-token.Done():
		if err := token.Error(); err != nil {
			return fmt.Errorf("publish frame-processing status: %w", err)
		}
		return nil
	}
}

func structToMap(value any) (map[string]interface{}, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal value: %w", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil, fmt.Errorf("decode value map: %w", err)
	}
	return result, nil
}

func NewSender(config models.FrameProcessing) (*Sender, error) {
	if config.Token == "" {
		return nil, errors.New("frameProcessing.token is required")
	}
	endpoint, err := url.ParseRequestURI(config.Endpoint)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" {
		return nil, errors.New("frameProcessing.endpoint must be an absolute HTTP or HTTPS URL")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if os.Getenv("AGENT_TLS_INSECURE") == "true" {
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{}
		}
		transport.TLSClientConfig.InsecureSkipVerify = true
	}
	return &Sender{
		endpoint: endpoint.String(),
		token:    config.Token,
		client: &http.Client{
			Transport: transport,
			Timeout:   time.Duration(config.RequestTimeoutSeconds) * time.Second,
		},
	}, nil
}

func (s *Sender) Submit(ctx context.Context, frame Frame) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	metadataHeader := make(textproto.MIMEHeader)
	metadataHeader.Set("Content-Disposition", `form-data; name="metadata"`)
	metadataHeader.Set("Content-Type", "application/json")
	metadataPart, err := writer.CreatePart(metadataHeader)
	if err != nil {
		return fmt.Errorf("create metadata part: %w", err)
	}
	if err := json.NewEncoder(metadataPart).Encode(frame.Metadata); err != nil {
		return fmt.Errorf("encode metadata: %w", err)
	}
	frameHeader := make(textproto.MIMEHeader)
	frameHeader.Set("Content-Disposition", `form-data; name="frame"; filename="frame.jpg"`)
	frameHeader.Set("Content-Type", "image/jpeg")
	framePart, err := writer.CreatePart(frameHeader)
	if err != nil {
		return fmt.Errorf("create frame part: %w", err)
	}
	if _, err := framePart.Write(frame.JPEG); err != nil {
		return fmt.Errorf("write frame part: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close multipart body: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, &body)
	if err != nil {
		return fmt.Errorf("create frame request: %w", err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if s.token != "" {
		request.Header.Set("Authorization", "Bearer "+s.token)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return fmt.Errorf("submit frame: %w", err)
	}
	defer response.Body.Close()
	_, readErr := io.Copy(io.Discard, io.LimitReader(response.Body, maxResponseBodyBytes))
	if readErr != nil {
		return fmt.Errorf("read frame response: %w", readErr)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("frame processor returned %s", response.Status)
	}
	return nil
}

func Run(
	ctx context.Context,
	cursor *packets.QueueCursor,
	decoder Decoder,
	config models.FrameProcessing,
	deviceID string,
	stream string,
	observer Observer,
) error {
	if config.Enabled != "true" {
		return nil
	}
	if cursor == nil || decoder == nil {
		return errors.New("frame processing requires a packet cursor and decoder")
	}
	if deviceID == "" {
		return errors.New("frame processing requires a device ID")
	}
	if err := validateConfig(config); err != nil {
		return err
	}
	sender, err := NewSender(config)
	if err != nil {
		return err
	}
	if observer != nil {
		observer.SetFrameProcessingConfigured(true)
		defer observer.SetFrameProcessingConfigured(false)
	}

	frames := make(chan Frame, config.PeriodicQueueCapacity)
	samplerDone := make(chan error, 1)
	go func() {
		samplerDone <- sample(ctx, cursor, decoder, config, deviceID, stream, frames, observer)
		close(frames)
	}()

	for {
		select {
		case <-ctx.Done():
			<-samplerDone
			return nil
		case err := <-samplerDone:
			return normalizeCancellation(ctx, err)
		case frame, ok := <-frames:
			if !ok {
				return normalizeCancellation(ctx, <-samplerDone)
			}
			if observer != nil {
				observer.SetFrameProcessingQueueDepth(len(frames))
			}
			if err := sender.Submit(ctx, frame); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				if observer != nil {
					observer.RecordFrameProcessingFailure()
					observer.SetFrameProcessingQueueDepth(len(frames))
				}
				log.WithError(err).WithFields(log.Fields{
					"component": "frame_processing",
					"device_id": deviceID,
					"event":     "frame_submission_failed",
					"frame_id":  frame.Metadata.FrameID,
				}).Warn("Failed to submit frame for processing")
				continue
			}
			if observer != nil {
				observer.RecordFrameProcessingSuccess(time.Now())
				observer.SetFrameProcessingQueueDepth(len(frames))
			}
		}
	}
}

func RunRequested(
	ctx context.Context,
	decoder Decoder,
	config models.FrameProcessing,
	deviceID string,
	stream string,
	requests <-chan models.FrameProcessingWork,
	statusPublisher StatusPublisher,
	observer Observer,
) error {
	if config.Enabled != "true" {
		return nil
	}
	if decoder == nil || requests == nil {
		return errors.New("requested frame processing requires a decoder and request channel")
	}
	if deviceID == "" {
		return errors.New("requested frame processing requires a device ID")
	}
	if err := validateConfig(config); err != nil {
		return err
	}
	sender, err := NewSender(config)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case work, ok := <-requests:
			if !ok {
				return nil
			}
			request := work.Request
			if work.Cursor == nil {
				publishStatus(ctx, statusPublisher, request, deviceID, "", "failed", true, "capture cursor is unavailable")
				continue
			}
			if request.ExpiresAt <= time.Now().UnixMilli() {
				publishStatus(ctx, statusPublisher, request, deviceID, "", "expired", false, "capture request expired")
				continue
			}
			packet, err := nextKeyframe(ctx, work.Cursor, request.ExpiresAt)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				publishStatus(ctx, statusPublisher, request, deviceID, "", "expired", false, "no keyframe before request expiry")
				continue
			}
			frame, err := prepareRequestedFrame(packet, decoder, config, deviceID, stream, request, time.Now())
			if err != nil {
				if observer != nil {
					observer.RecordFrameProcessingFailure()
				}
				publishStatus(ctx, statusPublisher, request, deviceID, "", "failed", true, "failed to prepare frame")
				continue
			}
			if err := sender.Submit(ctx, frame); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				if observer != nil {
					observer.RecordFrameProcessingFailure()
				}
				publishStatus(ctx, statusPublisher, request, deviceID, frame.Metadata.FrameID, "failed", true, "frame submission failed")
				continue
			}
			if observer != nil {
				observer.RecordFrameProcessingSuccess(time.Now())
			}
			publishStatus(ctx, statusPublisher, request, deviceID, frame.Metadata.FrameID, "submitted", false, "")
		}
	}
}

func sample(
	ctx context.Context,
	cursor *packets.QueueCursor,
	decoder Decoder,
	config models.FrameProcessing,
	deviceID string,
	stream string,
	frames chan Frame,
	observer Observer,
) error {
	interval := time.Duration(config.IntervalSeconds) * time.Second
	nextDeadline := time.Now().Add(interval)
	for {
		packet, err := cursor.ReadPacketContext(ctx)
		if err != nil {
			return err
		}
		now := time.Now()
		if len(packet.Data) == 0 || !packet.IsKeyFrame || now.Before(nextDeadline) {
			continue
		}
		for !nextDeadline.After(now) {
			nextDeadline = nextDeadline.Add(interval)
		}
		if observer != nil {
			observer.RecordFrameProcessingSample()
		}
		frame, err := prepareFrame(packet, decoder, config, deviceID, stream, now)
		if err != nil {
			if observer != nil {
				observer.RecordFrameProcessingFailure()
			}
			log.WithError(err).WithFields(log.Fields{
				"component": "frame_processing",
				"event":     "frame_preparation_failed",
				"stream":    stream,
			}).Warn("Failed to prepare frame for processing")
			continue
		}
		dropped := enqueueLatest(frames, frame)
		if observer != nil {
			observer.RecordFrameProcessingQueued(len(frames), dropped)
		}
	}
}

func prepareFrame(packet packets.Packet, decoder Decoder, config models.FrameProcessing, deviceID, stream string, now time.Time) (Frame, error) {
	frameID, err := uuid.NewV4()
	if err != nil {
		return Frame{}, fmt.Errorf("generate frame ID: %w", err)
	}
	return prepareFrameWithIdentity(packet, decoder, config, deviceID, stream, "periodic-"+frameID.String(), frameID.String(), config.Profile, "", now)
}

func prepareRequestedFrame(packet packets.Packet, decoder Decoder, config models.FrameProcessing, deviceID, stream string, request models.FrameProcessingRequest, now time.Time) (Frame, error) {
	frameID, err := uuid.NewV4()
	if err != nil {
		return Frame{}, fmt.Errorf("generate frame ID: %w", err)
	}
	return prepareFrameWithIdentity(packet, decoder, config, deviceID, stream, request.RequestID, frameID.String(), request.ProcessingProfile, request.TraceID, now)
}

func prepareFrameWithIdentity(packet packets.Packet, decoder Decoder, config models.FrameProcessing, deviceID, stream, requestID, frameID, profile, traceID string, now time.Time) (Frame, error) {
	decoded, err := decoder.DecodePacket(packet)
	if err != nil {
		return Frame{}, fmt.Errorf("decode keyframe: %w", err)
	}
	resized, err := utils.ResizeImage(&decoded, uint(config.Width), uint(config.Height))
	if err != nil {
		return Frame{}, fmt.Errorf("resize keyframe: %w", err)
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, *resized, &jpeg.Options{Quality: config.JPEGQuality}); err != nil {
		return Frame{}, fmt.Errorf("encode keyframe: %w", err)
	}
	if int64(encoded.Len()) > config.MaxFrameBytes {
		return Frame{}, fmt.Errorf("encoded keyframe exceeds frameProcessing.maxFrameBytes (%d)", config.MaxFrameBytes)
	}
	capturedAt := packet.CurrentTime
	if capturedAt <= 0 {
		capturedAt = now.UnixMilli()
	}
	bounds := (*resized).Bounds()
	return Frame{
		Metadata: Metadata{
			SchemaVersion:     schemaVersion,
			RequestID:         requestID,
			FrameID:           frameID,
			DeviceID:          deviceID,
			CapturedAt:        capturedAt,
			ExpiresAt:         now.Add(time.Duration(config.FrameTTLSeconds) * time.Second).UnixMilli(),
			ProcessingProfile: profile,
			SourceStream:      stream,
			Width:             bounds.Dx(),
			Height:            bounds.Dy(),
			TraceID:           traceID,
		},
		JPEG: encoded.Bytes(),
	}, nil
}

func nextKeyframe(ctx context.Context, cursor *packets.QueueCursor, expiresAt int64) (packets.Packet, error) {
	requestContext, cancel := context.WithDeadline(ctx, time.UnixMilli(expiresAt))
	defer cancel()
	for {
		packet, err := cursor.ReadPacketContext(requestContext)
		if err != nil {
			return packets.Packet{}, err
		}
		if packet.IsKeyFrame && len(packet.Data) > 0 {
			return packet, nil
		}
	}
}

func publishStatus(ctx context.Context, publisher StatusPublisher, request models.FrameProcessingRequest, deviceID, frameID, status string, retryable bool, message string) {
	if publisher == nil {
		return
	}
	err := publisher.Publish(ctx, models.FrameProcessingStatus{
		SchemaVersion: models.FrameProcessingSchemaVersion,
		RequestID:     request.RequestID,
		FrameID:       frameID,
		DeviceID:      deviceID,
		Status:        status,
		OccurredAt:    time.Now().UnixMilli(),
		Retryable:     retryable,
		Message:       message,
		TraceID:       request.TraceID,
	})
	if err != nil && ctx.Err() == nil {
		log.WithError(err).WithFields(log.Fields{
			"component":  "frame_processing",
			"event":      "status_publish_failed",
			"request_id": request.RequestID,
			"status":     status,
		}).Warn("Failed to publish frame-processing status")
	}
}

func enqueueLatest(frames chan Frame, frame Frame) bool {
	select {
	case frames <- frame:
		return false
	default:
	}
	select {
	case <-frames:
	default:
	}
	frames <- frame
	return true
}

func normalizeCancellation(ctx context.Context, err error) error {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func validateConfig(config models.FrameProcessing) error {
	if config.IntervalSeconds <= 0 {
		return errors.New("frameProcessing.intervalSeconds must be positive")
	}
	if config.Width <= 0 || config.Width > 8192 || config.Height < 0 || config.Height > 8192 {
		return errors.New("frameProcessing dimensions must be between 0 and 8192, with a positive width")
	}
	if config.JPEGQuality < 1 || config.JPEGQuality > 100 {
		return errors.New("frameProcessing.jpegQuality must be between 1 and 100")
	}
	if config.RequestTimeoutSeconds <= 0 || config.RequestTimeoutSeconds > 60 {
		return errors.New("frameProcessing.requestTimeoutSeconds must be between 1 and 60")
	}
	if config.FrameTTLSeconds <= 0 || config.FrameTTLSeconds > 3600 {
		return errors.New("frameProcessing.frameTtlSeconds must be between 1 and 3600")
	}
	if config.MaxFrameBytes <= 0 || config.MaxFrameBytes > 16<<20 {
		return errors.New("frameProcessing.maxFrameBytes must be between 1 and 16777216")
	}
	if config.PeriodicQueueCapacity <= 0 || config.PeriodicQueueCapacity > 64 {
		return errors.New("frameProcessing.periodicQueueCapacity must be between 1 and 64")
	}
	if config.Profile == "" {
		return errors.New("frameProcessing.profile is required")
	}
	return nil
}
