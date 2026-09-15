package service

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kerberos-io/agent/examples/frame-processor/contract"
	"github.com/kerberos-io/agent/examples/frame-processor/processor"
)

const maxMetadataBytes = 64 << 10

var errRequestTooLarge = errors.New("request exceeds maximum size")

type Publisher interface {
	PublishCaptureFrame(context.Context, string, contract.CaptureFrameCommand) error
	PublishRecordingWindow(context.Context, string, contract.RecordingWindowCommand) error
}

type Processor interface {
	Process(context.Context, contract.FrameMetadata, []byte) (processor.Decision, error)
}

type Config struct {
	APIToken         string
	MaxFrameBytes    int64
	MaxFrameTTL      time.Duration
	CommandTTL       time.Duration
	PreRollSeconds   int64
	EventClipSeconds int64
}

type Service struct {
	config    Config
	processor Processor
	publisher Publisher
	now       func() time.Time
	newID     func() string

	resultsMu sync.Mutex
	results   map[string]*frameResult
}

type frameResult struct {
	done      chan struct{}
	expiresAt int64
	response  FrameResponse
	err       error
}

type FrameResponse struct {
	SchemaVersion string `json:"schemaVersion"`
	RequestID     string `json:"requestId"`
	FrameID       string `json:"frameId"`
	Decision      string `json:"decision"`
	Reason        string `json:"reason"`
}

type FrameRequestResponse struct {
	SchemaVersion string   `json:"schemaVersion"`
	RequestID     string   `json:"requestId"`
	DeviceIDs     []string `json:"deviceIds"`
	Status        string   `json:"status"`
}

func New(config Config, frameProcessor Processor, publisher Publisher, newID func() string) *Service {
	if config.MaxFrameBytes <= 0 {
		config.MaxFrameBytes = 4 << 20
	}
	if config.CommandTTL <= 0 {
		config.CommandTTL = 30 * time.Second
	}
	if config.MaxFrameTTL <= 0 {
		config.MaxFrameTTL = 5 * time.Minute
	}
	if config.EventClipSeconds <= 0 {
		config.EventClipSeconds = 30
	}
	if newID == nil {
		newID = func() string { return fmt.Sprintf("request-%d", time.Now().UnixNano()) }
	}
	return &Service{
		config: config, processor: frameProcessor, publisher: publisher,
		now: time.Now, newID: newID, results: make(map[string]*frameResult),
	}
}

func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /v1/frames", s.authorize(s.handleFrame))
	mux.HandleFunc("POST /v1/frame-requests", s.authorize(s.handleFrameRequest))
	return mux
}

func (s *Service) authorize(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.config.APIToken == "" {
			writeError(w, http.StatusServiceUnavailable, "service authentication is not configured")
			return
		}
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(provided), []byte(s.config.APIToken)) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}

func (s *Service) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
}

func (s *Service) handleFrame(w http.ResponseWriter, r *http.Request) {
	metadata, frame, err := readFrame(w, r, s.config.MaxFrameBytes)
	if err != nil {
		if errors.Is(err, errRequestTooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	nowMillis := s.now().UnixMilli()
	if err := metadata.Validate(nowMillis); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if time.Duration(metadata.ExpiresAt-nowMillis)*time.Millisecond > s.config.MaxFrameTTL {
		writeError(w, http.StatusUnprocessableEntity, "expiresAt exceeds maximum frame TTL")
		return
	}
	imageConfig, _, err := image.DecodeConfig(bytes.NewReader(frame))
	if err != nil {
		writeError(w, http.StatusUnsupportedMediaType, "frame must be a valid JPEG")
		return
	}
	if imageConfig.Width != metadata.Width || imageConfig.Height != metadata.Height {
		writeError(w, http.StatusUnprocessableEntity, "frame dimensions do not match metadata")
		return
	}

	result, owner := s.beginFrame(metadata.FrameID, metadata.ExpiresAt, nowMillis)
	if !owner {
		select {
		case <-r.Context().Done():
			writeError(w, http.StatusRequestTimeout, "request cancelled")
			return
		case <-result.done:
		}
		if result.err != nil {
			writeError(w, http.StatusBadGateway, result.err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result.response)
		return
	}

	response, processErr := s.processFrame(r.Context(), metadata, frame)
	s.finishFrame(result, response, processErr)
	if processErr != nil {
		writeError(w, http.StatusBadGateway, processErr.Error())
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Service) processFrame(ctx context.Context, metadata contract.FrameMetadata, frame []byte) (FrameResponse, error) {
	decision, err := s.processor.Process(ctx, metadata, frame)
	if err != nil {
		return FrameResponse{}, fmt.Errorf("process frame: %w", err)
	}
	response := FrameResponse{
		SchemaVersion: contract.SchemaVersion,
		RequestID:     metadata.RequestID,
		FrameID:       metadata.FrameID,
		Decision:      "no-event",
		Reason:        decision.Reason,
	}
	if !decision.Triggered {
		return response, nil
	}
	command := contract.RecordingWindowCommand{
		SchemaVersion:     contract.SchemaVersion,
		RequestID:         metadata.RequestID,
		FrameID:           metadata.FrameID,
		CapturedAt:        metadata.CapturedAt,
		PreRollSeconds:    s.config.PreRollSeconds,
		EventClipSeconds:  s.config.EventClipSeconds,
		ExpiresAt:         s.now().Add(s.config.CommandTTL).UnixMilli(),
		ProcessingProfile: metadata.ProcessingProfile,
		TraceID:           metadata.TraceID,
	}
	if err := command.Validate(s.now().UnixMilli()); err != nil {
		return FrameResponse{}, fmt.Errorf("build recording command: %w", err)
	}
	if err := s.publisher.PublishRecordingWindow(ctx, metadata.DeviceID, command); err != nil {
		return FrameResponse{}, fmt.Errorf("publish recording command: %w", err)
	}
	response.Decision = "event"
	return response, nil
}

func (s *Service) handleFrameRequest(w http.ResponseWriter, r *http.Request) {
	var request contract.FrameRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxMetadataBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request")
		return
	}
	if request.RequestID == "" {
		request.RequestID = s.newID()
	}
	nowMillis := s.now().UnixMilli()
	if err := request.Validate(nowMillis); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if time.Duration(request.ExpiresAt-nowMillis)*time.Millisecond > s.config.MaxFrameTTL {
		writeError(w, http.StatusUnprocessableEntity, "expiresAt exceeds maximum frame TTL")
		return
	}
	for _, deviceID := range request.DeviceIDs {
		command := contract.CaptureFrameCommand{
			SchemaVersion:     contract.SchemaVersion,
			RequestID:         request.RequestID,
			ProcessingProfile: request.ProcessingProfile,
			ExpiresAt:         request.ExpiresAt,
			TraceID:           request.TraceID,
		}
		if err := s.publisher.PublishCaptureFrame(r.Context(), deviceID, command); err != nil {
			writeError(w, http.StatusBadGateway, "failed to publish capture command")
			return
		}
	}
	writeJSON(w, http.StatusAccepted, FrameRequestResponse{
		SchemaVersion: contract.SchemaVersion,
		RequestID:     request.RequestID,
		DeviceIDs:     request.DeviceIDs,
		Status:        "accepted",
	})
}

func (s *Service) beginFrame(frameID string, expiresAt, nowMillis int64) (*frameResult, bool) {
	s.resultsMu.Lock()
	defer s.resultsMu.Unlock()
	for id, result := range s.results {
		if result.expiresAt <= nowMillis {
			delete(s.results, id)
		}
	}
	if result, ok := s.results[frameID]; ok {
		return result, false
	}
	result := &frameResult{done: make(chan struct{}), expiresAt: expiresAt}
	s.results[frameID] = result
	delay := time.Duration(expiresAt-nowMillis) * time.Millisecond
	time.AfterFunc(delay, func() {
		s.resultsMu.Lock()
		if s.results[frameID] == result {
			delete(s.results, frameID)
		}
		s.resultsMu.Unlock()
	})
	return result, true
}

func (s *Service) finishFrame(result *frameResult, response FrameResponse, err error) {
	s.resultsMu.Lock()
	result.response = response
	result.err = err
	close(result.done)
	s.resultsMu.Unlock()
}

func readFrame(w http.ResponseWriter, r *http.Request, maxFrameBytes int64) (contract.FrameMetadata, []byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFrameBytes+maxMetadataBytes)
	reader, err := r.MultipartReader()
	if err != nil {
		return contract.FrameMetadata{}, nil, errors.New("content type must be multipart/form-data")
	}
	var metadata contract.FrameMetadata
	var frame []byte
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				return contract.FrameMetadata{}, nil, errRequestTooLarge
			}
			return contract.FrameMetadata{}, nil, errors.New("invalid multipart body")
		}
		switch part.FormName() {
		case "metadata":
			if err := decodeMetadataPart(part, &metadata); err != nil {
				return contract.FrameMetadata{}, nil, err
			}
		case "frame":
			if part.Header.Get("Content-Type") != "image/jpeg" {
				return contract.FrameMetadata{}, nil, errors.New("frame content type must be image/jpeg")
			}
			frame, err = io.ReadAll(io.LimitReader(part, maxFrameBytes+1))
			if err != nil || int64(len(frame)) > maxFrameBytes {
				return contract.FrameMetadata{}, nil, errRequestTooLarge
			}
		}
	}
	if metadata.FrameID == "" || len(frame) == 0 {
		return contract.FrameMetadata{}, nil, errors.New("metadata and frame parts are required")
	}
	return metadata, frame, nil
}

func decodeMetadataPart(part *multipart.Part, target *contract.FrameMetadata) error {
	value, err := io.ReadAll(io.LimitReader(part, maxMetadataBytes+1))
	if err != nil || len(value) > maxMetadataBytes {
		return errRequestTooLarge
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("invalid metadata JSON")
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"schemaVersion": contract.SchemaVersion,
		"error":         message,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("failed to encode HTTP response", "error", err)
	}
}
