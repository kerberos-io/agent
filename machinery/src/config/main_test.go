package config

import (
	"context"
	"errors"
	"os"
	"reflect"
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

func TestApplyAgentEnvVarsRTSPTransport(t *testing.T) {
	t.Setenv("AGENT_CAPTURE_IPCAMERA_RTSP_TRANSPORT", "udp")
	configuration := &models.Configuration{}
	initConfigPointers(&configuration.Config)

	applyAgentEnvVars(configuration, "", false)

	if got := configuration.Config.Capture.IPCamera.RTSPTransport; got != "udp" {
		t.Fatalf("RTSPTransport = %q, want udp", got)
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

func TestParseRegionPolygons(t *testing.T) {
	square := []models.Coordinate{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10}}
	triangle := []models.Coordinate{{X: 20, Y: 20}, {X: 30, Y: 20}, {X: 30, Y: 30}}

	tests := []struct {
		name  string
		value string
		want  []models.Polygon
	}{
		{
			name:  "single polygon parses as before",
			value: "0,0;10,0;10,10;0,10",
			want:  []models.Polygon{{ID: "0", Coordinates: square}},
		},
		{
			name:  "multiple polygons separated by pipe",
			value: "0,0;10,0;10,10;0,10|20,20;30,20;30,30",
			want:  []models.Polygon{{ID: "0", Coordinates: square}, {ID: "1", Coordinates: triangle}},
		},
		{
			name:  "issue example",
			value: "64,36;576,36;608,324;32,324|700,40;900,40;900,300",
			want: []models.Polygon{
				{ID: "0", Coordinates: []models.Coordinate{{X: 64, Y: 36}, {X: 576, Y: 36}, {X: 608, Y: 324}, {X: 32, Y: 324}}},
				{ID: "1", Coordinates: []models.Coordinate{{X: 700, Y: 40}, {X: 900, Y: 40}, {X: 900, Y: 300}}},
			},
		},
		{
			name:  "empty parts are skipped and ids stay sequential",
			value: "|0,0;10,0;10,10;0,10||20,20;30,20;30,30|",
			want:  []models.Polygon{{ID: "0", Coordinates: square}, {ID: "1", Coordinates: triangle}},
		},
		{
			name:  "parts with fewer than 3 points are skipped",
			value: "1,1;2,2|0,0;10,0;10,10;0,10|5,5",
			want:  []models.Polygon{{ID: "0", Coordinates: square}},
		},
		{
			name:  "invalid points are ignored before counting",
			value: "0,0;x,1;10,0;10;10,10;0,10;",
			want:  []models.Polygon{{ID: "0", Coordinates: square}},
		},
		{
			name:  "whitespace is trimmed",
			value: " 0, 0 ; 10,0;10 ,10; 0,10 | 20,20; 30,20 ;30,30 ",
			want:  []models.Polygon{{ID: "0", Coordinates: square}, {ID: "1", Coordinates: triangle}},
		},
		{
			name:  "decimal coordinates",
			value: "0.5,0.5;10.25,0;10,10.75",
			want:  []models.Polygon{{ID: "0", Coordinates: []models.Coordinate{{X: 0.5, Y: 0.5}, {X: 10.25, Y: 0}, {X: 10, Y: 10.75}}}},
		},
		{name: "empty value", value: "", want: []models.Polygon{}},
		{name: "only separators", value: "||", want: []models.Polygon{}},
		{name: "single polygon with too few points", value: "0,0;10,10", want: []models.Polygon{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := parseRegionPolygons(test.value)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("parseRegionPolygons(%q) = %+v, want %+v", test.value, got, test.want)
			}
		})
	}
}

func TestApplyAgentEnvVarsRegionPolygon(t *testing.T) {
	for _, prefix := range []string{"", "GLOBAL_"} {
		t.Run("prefix="+prefix, func(t *testing.T) {
			t.Setenv(prefix+"AGENT_REGION_POLYGON", "0,0;10,0;10,10;0,10|20,20;30,20;30,30")
			configuration := &models.Configuration{}
			initConfigPointers(&configuration.Config)

			applyAgentEnvVars(configuration, prefix, false)

			polygons := configuration.Config.Region.Polygon
			if len(polygons) != 2 {
				t.Fatalf("got %d polygons, want 2: %+v", len(polygons), polygons)
			}
			if polygons[0].ID != "0" || polygons[1].ID != "1" {
				t.Fatalf("got ids %q, %q, want \"0\", \"1\"", polygons[0].ID, polygons[1].ID)
			}
			if len(polygons[0].Coordinates) != 4 || len(polygons[1].Coordinates) != 3 {
				t.Fatalf("got %d and %d points, want 4 and 3", len(polygons[0].Coordinates), len(polygons[1].Coordinates))
			}
		})
	}
}
