package cloud

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHeartbeatFailureLogIncludesHubResponse(t *testing.T) {
	response := &http.Response{
		StatusCode: http.StatusBadRequest,
		Status:     "400 Bad Request",
		Body:       io.NopCloser(strings.NewReader(`{"error":"invalid heartbeat"}`)),
	}

	responseBody, truncated, err := readHeartbeatResponseBody(response)
	if err != nil {
		t.Fatalf("readHeartbeatResponseBody() error = %v", err)
	}
	message := formatHeartbeatFailureLog(response, nil, responseBody, truncated, nil, 125*time.Millisecond)

	for _, expected := range []string{
		"status_code=400",
		`status="400 Bad Request"`,
		"duration=125ms",
		`response_body="{\"error\":\"invalid heartbeat\"}"`,
	} {
		if !strings.Contains(message, expected) {
			t.Errorf("formatHeartbeatFailureLog() = %q, want it to contain %q", message, expected)
		}
	}
}

func TestReadHeartbeatResponseBodyTruncatesLargeBody(t *testing.T) {
	response := &http.Response{
		Body: io.NopCloser(strings.NewReader(strings.Repeat("x", heartbeatResponseBodyLogLimit+1))),
	}

	body, truncated, err := readHeartbeatResponseBody(response)
	if err != nil {
		t.Fatalf("readHeartbeatResponseBody() error = %v", err)
	}
	if !truncated {
		t.Fatal("readHeartbeatResponseBody() truncated = false, want true")
	}
	if len(body) != heartbeatResponseBodyLogLimit {
		t.Fatalf("len(body) = %d, want %d", len(body), heartbeatResponseBodyLogLimit)
	}
}
