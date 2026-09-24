package config

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/kerberos-io/agent/machinery/src/database"
	"github.com/kerberos-io/agent/machinery/src/models"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestApplyAgentEnvVarsDeploymentName(t *testing.T) {
	tests := []struct {
		name           string
		prefix         string
		deploymentName string
		agentName      string
		initialName    string
		initialDisplay string
		wantName       string
		wantDisplay    string
	}{
		{name: "deployment fallback", deploymentName: "camera-1", wantName: "camera-1", wantDisplay: "camera-1"},
		{name: "explicit display name", deploymentName: "camera-1", agentName: "Front Door", wantName: "camera-1", wantDisplay: "Front Door"},
		{name: "existing display name", deploymentName: "camera-1", initialDisplay: "Front Door", wantName: "camera-1", wantDisplay: "Front Door"},
		{name: "deployment overrides bundled identity", deploymentName: "camera-1", initialName: "default", wantName: "camera-1", wantDisplay: "camera-1"},
		{name: "no deployment", initialName: "existing", initialDisplay: "Existing Camera", wantName: "existing", wantDisplay: "Existing Camera"},
		{name: "standalone display name", agentName: "Front Door", wantDisplay: "Front Door"},
		{name: "global layer excludes deployment", prefix: "GLOBAL_", deploymentName: "camera-1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("DEPLOYMENT_NAME", test.deploymentName)
			t.Setenv(test.prefix+"AGENT_NAME", test.agentName)
			if test.agentName == "" {
				if err := os.Unsetenv(test.prefix + "AGENT_NAME"); err != nil {
					t.Fatal(err)
				}
			}
			configuration := &models.Configuration{Config: models.Config{
				Name:         test.initialName,
				FriendlyName: test.initialDisplay,
			}}
			initConfigPointers(&configuration.Config)

			applyAgentEnvVars(configuration, test.prefix, false)

			if got := configuration.Config.Name; got != test.wantName {
				t.Errorf("Name = %q, want %q", got, test.wantName)
			}
			if got := configuration.Config.FriendlyName; got != test.wantDisplay {
				t.Errorf("FriendlyName = %q, want %q", got, test.wantDisplay)
			}
		})
	}
}

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

func TestApplyAgentEnvVarsVaultCustomHeaders(t *testing.T) {
	t.Setenv("AGENT_KERBEROSVAULT_CUSTOM_HEADERS", `{"site_id":"site-1"}`)
	t.Setenv("AGENT_KERBEROSVAULT_SECONDARY_CUSTOM_HEADERS", `{"site_id":"site-2"}`)
	configuration := &models.Configuration{}
	initConfigPointers(&configuration.Config)

	applyAgentEnvVars(configuration, "", false)

	if got := configuration.Config.KStorage.CustomHeaders; got != `{"site_id":"site-1"}` {
		t.Fatalf("primary custom headers = %q", got)
	}
	if got := configuration.Config.KStorageSecondary.CustomHeaders; got != `{"site_id":"site-2"}` {
		t.Fatalf("secondary custom headers = %q", got)
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
