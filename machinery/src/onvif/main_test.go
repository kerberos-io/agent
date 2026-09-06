package onvif

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
)

func TestHandleONVIFResponseDoesNotLogBody(t *testing.T) {
	logger := log.StandardLogger()
	originalOutput := logger.Out
	originalFormatter := logger.Formatter
	originalLevel := logger.Level
	defer func() {
		logger.SetOutput(originalOutput)
		logger.SetFormatter(originalFormatter)
		logger.SetLevel(originalLevel)
	}()

	var output bytes.Buffer
	logger.SetOutput(&output)
	logger.SetFormatter(&log.JSONFormatter{})
	logger.SetLevel(log.DebugLevel)

	response := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"credential":"do-not-log"}`)),
	}
	if err := handleONVIFResponse("test_operation", response, nil); err != nil {
		t.Fatalf("handleONVIFResponse() error = %v", err)
	}
	if strings.Contains(output.String(), "do-not-log") {
		t.Fatalf("ONVIF response log exposed response body: %q", output.String())
	}
	if !strings.Contains(output.String(), `"response_bytes":27`) {
		t.Fatalf("ONVIF response log omitted response byte count: %q", output.String())
	}
}
