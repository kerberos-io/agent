package http

import (
	"bytes"
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

func TestRequestLoggerEmitsStructuredFields(t *testing.T) {
	originalOutput := log.StandardLogger().Out
	originalFormatter := log.StandardLogger().Formatter
	originalLevel := log.GetLevel()
	defer func() {
		log.SetOutput(originalOutput)
		log.SetFormatter(originalFormatter)
		log.SetLevel(originalLevel)
	}()

	var output bytes.Buffer
	log.SetOutput(&output)
	log.SetFormatter(&log.JSONFormatter{})
	log.SetLevel(log.InfoLevel)
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(requestLogger())
	router.GET("/health", func(c *gin.Context) {
		c.Status(stdhttp.StatusNoContent)
	})

	response := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodGet, "/health?token=do-not-log", nil)
	router.ServeHTTP(response, request)

	var entry map[string]interface{}
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatalf("decode request log: %v; output=%q", err, output.String())
	}
	for key, want := range map[string]interface{}{
		"component": "http",
		"event":     "request_completed",
		"method":    stdhttp.MethodGet,
		"path":      "/health",
		"status":    float64(stdhttp.StatusNoContent),
	} {
		if got := entry[key]; got != want {
			t.Fatalf("%s = %v, want %v", key, got, want)
		}
	}
	if bytes.Contains(output.Bytes(), []byte("do-not-log")) {
		t.Fatalf("request log exposed query parameters: %q", output.String())
	}
}
