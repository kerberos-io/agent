package conditions

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/kerberos-io/agent/machinery/src/models"
)

func TestIsValidUriResponseReusesClientAndEncodesPayload(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if got := request.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		var payload struct {
			CameraID   string `json:"camera_id"`
			CameraName string `json:"camera_name"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if payload.CameraID != "camera-1" || payload.CameraName != `Front "Door"` {
			t.Errorf("payload = %+v", payload)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	configuration := &models.Configuration{}
	configuration.Config.ConditionURI = server.URL
	configuration.Config.Key = "camera-1"
	configuration.Config.FriendlyName = `Front "Door"`

	for i := 0; i < 2; i++ {
		if !IsValidUriResponse(configuration) {
			t.Fatal("IsValidUriResponse() = false, want true")
		}
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}
}

func TestIsValidUriResponseRejectsNonOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	configuration := &models.Configuration{}
	configuration.Config.ConditionURI = server.URL
	if IsValidUriResponse(configuration) {
		t.Fatal("IsValidUriResponse() = true, want false")
	}
}
