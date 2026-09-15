package config

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kerberos-io/agent/machinery/src/database"
	"github.com/kerberos-io/agent/machinery/src/models"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestApplyAgentEnvVarsPixelChangeThresholdDefault(t *testing.T) {
	tests := []struct {
		name      string
		threshold *int
		want      int
	}{
		{name: "missing", want: 150},
		{name: "legacy zero", threshold: intPointer(0), want: 150},
		{name: "negative", threshold: intPointer(-1), want: 150},
		{name: "positive", threshold: intPointer(275), want: 275},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configuration := &models.Configuration{}
			configuration.Config.Capture.PixelChangeThreshold = test.threshold

			applyAgentEnvVars(configuration, "TEST_", true)

			if configuration.Config.Capture.PixelChangeThreshold == nil {
				t.Fatal("PixelChangeThreshold is nil after applying defaults")
			}
			if got := *configuration.Config.Capture.PixelChangeThreshold; got != test.want {
				t.Fatalf("PixelChangeThreshold = %d, want %d", got, test.want)
			}
		})
	}
}

func intPointer(value int) *int {
	return &value
}

func TestApplyAgentEnvVarsFrameProcessing(t *testing.T) {
	t.Setenv("AGENT_FRAME_PROCESSING_ENABLED", "true")
	t.Setenv("AGENT_FRAME_PROCESSING_ENDPOINT", "http://processor:8080/v1/frames")
	t.Setenv("AGENT_FRAME_PROCESSING_TOKEN", "secret")
	t.Setenv("AGENT_FRAME_PROCESSING_PROFILE", "always-trigger")
	t.Setenv("AGENT_FRAME_PROCESSING_ALLOW_REQUESTED_FRAMES", "true")
	t.Setenv("AGENT_FRAME_PROCESSING_STREAM", "sub")
	t.Setenv("AGENT_FRAME_PROCESSING_INTERVAL_SECONDS", "15")
	t.Setenv("AGENT_FRAME_PROCESSING_WIDTH", "320")
	t.Setenv("AGENT_FRAME_PROCESSING_HEIGHT", "180")
	t.Setenv("AGENT_FRAME_PROCESSING_JPEG_QUALITY", "80")
	t.Setenv("AGENT_FRAME_PROCESSING_REQUEST_TIMEOUT_SECONDS", "7")
	t.Setenv("AGENT_FRAME_PROCESSING_FRAME_TTL_SECONDS", "45")
	t.Setenv("AGENT_FRAME_PROCESSING_MAX_FRAME_BYTES", "2097152")
	t.Setenv("AGENT_FRAME_PROCESSING_PERIODIC_QUEUE_CAPACITY", "2")

	configuration := &models.Configuration{}
	initConfigPointers(&configuration.Config)
	applyAgentEnvVars(configuration, "", true)

	got := configuration.Config.FrameProcessing
	if got == nil {
		t.Fatal("FrameProcessing is nil")
	}
	if got.Enabled != "true" || got.Endpoint != "http://processor:8080/v1/frames" || got.Token != "secret" {
		t.Fatalf("FrameProcessing identity = %+v", got)
	}
	if got.Profile != "always-trigger" || got.AllowRequestedFrames != "true" || got.Stream != "sub" || got.IntervalSeconds != 15 {
		t.Fatalf("FrameProcessing schedule = %+v", got)
	}
	if got.Width != 320 || got.Height != 180 || got.JPEGQuality != 80 {
		t.Fatalf("FrameProcessing image = %+v", got)
	}
	if got.RequestTimeoutSeconds != 7 || got.FrameTTLSeconds != 45 || got.MaxFrameBytes != 2097152 || got.PeriodicQueueCapacity != 2 {
		t.Fatalf("FrameProcessing delivery = %+v", got)
	}
}

func TestApplyAgentEnvVarsFrameProcessingDefaults(t *testing.T) {
	configuration := &models.Configuration{}
	initConfigPointers(&configuration.Config)
	applyAgentEnvVars(configuration, "", true)

	got := configuration.Config.FrameProcessing
	if got.Profile != "never-trigger" || got.Stream != "auto" || got.IntervalSeconds != 10 {
		t.Fatalf("FrameProcessing defaults = %+v", got)
	}
	if got.Width != 640 || got.Height != 0 || got.JPEGQuality != 70 {
		t.Fatalf("FrameProcessing image defaults = %+v", got)
	}
	if got.RequestTimeoutSeconds != 5 || got.FrameTTLSeconds != 30 || got.MaxFrameBytes != 4<<20 || got.PeriodicQueueCapacity != 1 {
		t.Fatalf("FrameProcessing delivery defaults = %+v", got)
	}
}

func TestOverrideWithEnvironmentVariablesInheritsGlobalFrameProcessing(t *testing.T) {
	t.Setenv("GLOBAL_AGENT_FRAME_PROCESSING_ENABLED", "true")
	t.Setenv("GLOBAL_AGENT_FRAME_PROCESSING_ENDPOINT", "https://processor.example/v1/frames")
	t.Setenv("GLOBAL_AGENT_FRAME_PROCESSING_PROFILE", "never-trigger")

	configuration := &models.Configuration{}
	OverrideWithEnvironmentVariables(configuration)

	got := configuration.Config.FrameProcessing
	if got == nil || got.Enabled != "true" || got.Endpoint != "https://processor.example/v1/frames" {
		t.Fatalf("effective FrameProcessing = %+v", got)
	}
	if configuration.CustomConfig.FrameProcessing == nil || configuration.CustomConfig.FrameProcessing.Enabled != "" {
		t.Fatalf("custom FrameProcessing unexpectedly overrides global config: %+v", configuration.CustomConfig.FrameProcessing)
	}
}

func TestNewFactoryConfigReadContextUsesDatabaseTimeout(t *testing.T) {
	ctx, cancel := newFactoryConfigReadContext()
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected read context deadline")
	}

	remaining := time.Until(deadline)
	if remaining <= 0 {
		t.Fatalf("deadline already expired: %v", remaining)
	}
	if remaining > database.TIMEOUT {
		t.Fatalf("remaining deadline = %v, want <= %v", remaining, database.TIMEOUT)
	}
	if remaining < database.TIMEOUT-time.Second {
		t.Fatalf("remaining deadline = %v, want close to %v", remaining, database.TIMEOUT)
	}
}

func TestFactoryConfigRetryableErrors(t *testing.T) {
	if !isRetryableFactoryConfigReadError(context.DeadlineExceeded) {
		t.Fatal("context deadline should be retryable")
	}
	if isRetryableFactoryConfigReadError(mongo.ErrNoDocuments) {
		t.Fatal("missing configuration should not retry")
	}
	if isRetryableFactoryConfigReadError(errors.New("invalid BSON")) {
		t.Fatal("decode errors should not retry")
	}
}
