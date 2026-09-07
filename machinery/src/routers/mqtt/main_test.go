package mqtt

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/kerberos-io/agent/machinery/src/models"
)

func TestConfigureMQTTRequiresHubKey(t *testing.T) {
	configuration := &models.Configuration{Config: models.Config{Key: "agent-key"}}

	if client := ConfigureMQTT("", configuration, &models.Communication{}); client != nil {
		t.Fatal("ConfigureMQTT() returned a client without a Hub key")
	}
}

func TestConfigureMQTTRequiresAgentKey(t *testing.T) {
	configuration := &models.Configuration{Config: models.Config{HubKey: "hub-key"}}

	if client := ConfigureMQTT("", configuration, &models.Communication{}); client != nil {
		t.Fatal("ConfigureMQTT() returned a client without an Agent key")
	}
}

func TestEnqueueLatestAudioReplacesOldestFrameWhenFull(t *testing.T) {
	audioChannel := make(chan models.AudioDataPartial, 2)
	audioChannel <- models.AudioDataPartial{Timestamp: 1}
	audioChannel <- models.AudioDataPartial{Timestamp: 2}

	dropped := enqueueLatestAudio(audioChannel, models.AudioDataPartial{Timestamp: 3})
	if !dropped {
		t.Fatal("enqueueLatestAudio() dropped = false, want true")
	}

	first := <-audioChannel
	second := <-audioChannel
	if first.Timestamp != 2 || second.Timestamp != 3 {
		t.Fatalf("queued timestamps = (%d, %d), want (2, 3)", first.Timestamp, second.Timestamp)
	}
}

func TestEnqueueLatestAudioDoesNotBlockNilChannel(t *testing.T) {
	done := make(chan struct{})
	go func() {
		enqueueLatestAudio(nil, models.AudioDataPartial{Timestamp: 1})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("enqueueLatestAudio() blocked on a nil channel")
	}
}

func TestRemoteAccessRequiresExplicitOptIn(t *testing.T) {
	previous, present := os.LookupEnv(remoteAccessEnvironment)
	t.Cleanup(func() {
		if present {
			_ = os.Setenv(remoteAccessEnvironment, previous)
		} else {
			_ = os.Unsetenv(remoteAccessEnvironment)
		}
	})

	_ = os.Unsetenv(remoteAccessEnvironment)
	if remoteAccessEnabled() {
		t.Fatal("remoteAccessEnabled() = true without opt-in")
	}
	_ = os.Setenv(remoteAccessEnvironment, "true")
	if !remoteAccessEnabled() {
		t.Fatal("remoteAccessEnabled() = false after opt-in")
	}
}

func TestNormalizeTerminalSize(t *testing.T) {
	rows, columns := normalizeTerminalSize(0, 0)
	if rows != 24 || columns != 80 {
		t.Fatalf("normalizeTerminalSize(0, 0) = (%d, %d), want (24, 80)", rows, columns)
	}

	rows, columns = normalizeTerminalSize(500, 500)
	if rows != 200 || columns != 400 {
		t.Fatalf("normalizeTerminalSize(500, 500) = (%d, %d), want (200, 400)", rows, columns)
	}
}

func TestDecodeRemotePayloadRejectsMissingSession(t *testing.T) {
	_, err := decodeRemotePayload(models.Payload{Value: map[string]interface{}{
		"kind": "shell",
	}})
	if err == nil {
		t.Fatal("decodeRemotePayload() accepted a missing session id")
	}
}

func TestRemoteSessionOpenRejectsUnprovenEncryption(t *testing.T) {
	previous, present := os.LookupEnv(remoteAccessEnvironment)
	t.Cleanup(func() {
		if present {
			_ = os.Setenv(remoteAccessEnvironment, previous)
		} else {
			_ = os.Unsetenv(remoteAccessEnvironment)
		}
	})
	_ = os.Setenv(remoteAccessEnvironment, "true")

	// The listener passes false when an envelope merely claims to be hidden but
	// no ciphertext was successfully decrypted. The remote handler must reject it.
	if err := validateRemoteAccess(false); !errors.Is(err, errRemoteUnauthenticated) {
		t.Fatalf("validateRemoteAccess(false) error = %v, want %v", err, errRemoteUnauthenticated)
	}
}

func TestRemoteSessionReservationIsIdempotent(t *testing.T) {
	manager := newRemoteAccessManager()
	session := &remoteSession{id: "session-1", kind: "logs"}
	if _, err := manager.reserve(session); err != nil {
		t.Fatalf("first reserve() failed: %v", err)
	}
	if _, err := manager.reserve(session); !errors.Is(err, errRemoteSessionExists) {
		t.Fatalf("duplicate reserve() error = %v, want %v", err, errRemoteSessionExists)
	}
}
