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
