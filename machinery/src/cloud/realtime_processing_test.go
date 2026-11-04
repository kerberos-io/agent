package cloud

import (
	"testing"

	"github.com/kerberos-io/agent/machinery/src/models"
	log "github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
)

func TestHandleRealtimeProcessingWithoutMQTTClient(t *testing.T) {
	tests := []struct {
		name        string
		config      models.Config
		wantWarning bool
	}{
		{
			name:        "enabled without Hub credentials",
			config:      models.Config{RealtimeProcessing: "true"},
			wantWarning: true,
		},
		{
			name:   "disabled",
			config: models.Config{RealtimeProcessing: "false"},
		},
		{
			name:   "offline",
			config: models.Config{RealtimeProcessing: "true", Offline: "true"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hook := test.NewGlobal()
			defer hook.Reset()

			configuration := &models.Configuration{Config: tt.config}
			// No cursor or decoder should be accessed without an MQTT client.
			HandleRealtimeProcessing(nil, configuration, nil, nil, nil)

			warnings := 0
			for _, entry := range hook.AllEntries() {
				if entry.Level == log.WarnLevel {
					warnings++
					if entry.Message != "cloud.RealtimeProcessing(): stopping as MQTT client is not configured." {
						t.Errorf("unexpected warning: %s", entry.Message)
					}
				}
			}
			wantWarnings := 0
			if tt.wantWarning {
				wantWarnings = 1
			}
			if warnings != wantWarnings {
				t.Fatalf("warnings = %d, want %d", warnings, wantWarnings)
			}
		})
	}
}
