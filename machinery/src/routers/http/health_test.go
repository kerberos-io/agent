package http

import (
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"
	"time"

	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/utils"
)

func TestHealthCheckReturnsStandardPublicResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)

	middlewareConfig := JWTMiddleWare()
	authMiddleware, err := jwt.New(&middlewareConfig)
	if err != nil {
		t.Fatalf("initialize JWT middleware: %v", err)
	}

	now := time.Now()
	communication := &models.Communication{}
	communication.SetStreamConfigured(models.MainStream, true)
	communication.SetStreamConfigured(models.SubStream, true)
	communication.RecordStreamPackage(models.MainStream, 29.969, 1920, 1080, now.Add(-2*time.Second))
	communication.RecordStreamPackage(models.MainStream, 29.969, 1920, 1080, now.Add(-time.Second))
	communication.RecordStreamPackage(models.SubStream, 15, 640, 360, now.Add(-time.Second))
	communication.CameraConnected.Store(true)
	communication.MainStreamConnected.Store(true)
	communication.SubStreamConnected.Store(true)
	communication.SetHubConfigured(true)
	communication.RecordHubHeartbeatAttempt(now.Add(-2 * time.Second))
	communication.RecordHubHeartbeatSuccess(now.Add(-time.Second))

	router := gin.New()
	AddRoutes(router, authMiddleware, "", nil, communication, nil)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodGet, "/health", nil)
	router.ServeHTTP(response, request)

	if response.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, stdhttp.StatusOK, response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want %q", got, "no-store")
	}

	var body HealthResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode health response: %v; body=%s", err, response.Body.String())
	}

	if body.HTTPStatusCode != stdhttp.StatusOK {
		t.Errorf("httpStatusCode = %d, want %d", body.HTTPStatusCode, stdhttp.StatusOK)
	}
	if body.ApplicationStatusCode != healthApplicationStatus {
		t.Errorf("applicationStatusCode = %q, want %q", body.ApplicationStatusCode, healthApplicationStatus)
	}
	if body.EntityStatusCode != healthEntityStatus {
		t.Errorf("entityStatusCode = %q, want %q", body.EntityStatusCode, healthEntityStatus)
	}
	if body.Message != "Healthy" {
		t.Errorf("message = %q, want %q", body.Message, "Healthy")
	}
	if body.Metadata.ApplicationName != "agent" {
		t.Errorf("metadata.applicationName = %q, want %q", body.Metadata.ApplicationName, "agent")
	}
	if body.Metadata.ApplicationVersion != utils.VERSION {
		t.Errorf("metadata.applicationVersion = %q, want %q", body.Metadata.ApplicationVersion, utils.VERSION)
	}
	if body.Metadata.Timestamp <= 0 {
		t.Errorf("metadata.timestamp = %d, want a positive Unix timestamp", body.Metadata.Timestamp)
	}
	if body.Metadata.Path != "/health" {
		t.Errorf("metadata.path = %q, want %q", body.Metadata.Path, "/health")
	}
	health := body.Data.Health
	if health.Description != "Agent HTTP service is healthy" {
		t.Errorf("data.health.description = %q, want %q", health.Description, "Agent HTTP service is healthy")
	}
	if !health.CameraConnected {
		t.Error("data.health.cameraConnected = false, want true")
	}
	if health.MainStream.PackagesProcessed != 2 || health.MainStream.FPS != 29.97 {
		t.Errorf("data.health.mainStream = %+v, want 2 packages at 29.97 FPS", health.MainStream)
	}
	if health.MainStream.Resolution != (StreamResolution{Width: 1920, Height: 1080}) {
		t.Errorf("data.health.mainStream.resolution = %+v", health.MainStream.Resolution)
	}
	if health.SubStream.PackagesProcessed != 1 || health.SubStream.FPS != 15 {
		t.Errorf("data.health.subStream = %+v, want 1 package at 15 FPS", health.SubStream)
	}
	if health.SubStream.Resolution != (StreamResolution{Width: 640, Height: 360}) {
		t.Errorf("data.health.subStream.resolution = %+v", health.SubStream.Resolution)
	}
	if !health.Hub.Configured || !health.Hub.Connected {
		t.Errorf("data.health.hub = %+v, want configured and connected", health.Hub)
	}
}

func TestBuildHealthMarksStaleHubHeartbeatDisconnected(t *testing.T) {
	now := time.Unix(1_788_710_500, 0)
	communication := &models.Communication{}
	communication.SetHubConfigured(true)
	communication.RecordHubHeartbeatAttempt(now.Add(-4 * time.Minute))
	communication.RecordHubHeartbeatSuccess(now.Add(-4 * time.Minute))

	health := buildHealth(communication, now)
	if health.Hub.Connected {
		t.Fatalf("Hub health = %+v, want stale heartbeat to be disconnected", health.Hub)
	}
}
