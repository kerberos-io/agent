package mqtt

import (
	"crypto/tls"
	"errors"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/kerberos-io/agent/machinery/src/models"
)

func TestEnableMQTTConnectionDiagnostics(t *testing.T) {
	tests := []struct {
		name      string
		brokerURL string
		want      bool
	}{
		{name: "ActiveMQ TLS", brokerURL: "mqtt+ssl://broker.example:8883", want: true},
		{name: "TCP", brokerURL: "tcp://broker.example:1883", want: true},
		{name: "default TCP", brokerURL: "broker.example:1883", want: true},
		{name: "WebSocket", brokerURL: "wss://broker.example/mqtt", want: false},
		{name: "ActiveMQ failover", brokerURL: "failover:(mqtt+ssl://b-1.example:8883,mqtt+ssl://b-2.example:8883)", want: true},
		{name: "mixed TCP and WebSocket", brokerURL: "tcp://b-1.example:1883,wss://b-2.example/mqtt", want: false},
		{name: "empty", brokerURL: "", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := mqtt.NewClientOptions()
			enableMQTTConnectionDiagnostics(options, parseMQTTBrokers(test.brokerURL))
			if got := options.CustomOpenConnectionFn != nil; got != test.want {
				t.Fatalf("CustomOpenConnectionFn configured = %t, want %t", got, test.want)
			}
		})
	}
}

func TestOpenMQTTConnectionReturnsTCPError(t *testing.T) {
	options := *mqtt.NewClientOptions().SetConnectTimeout(100 * time.Millisecond)
	brokerURL, err := url.Parse("tcp://127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	connection, err := openMQTTConnection(brokerURL, options)
	if connection != nil {
		connection.Close()
		t.Fatal("openMQTTConnection() returned a connection for an unavailable endpoint")
	}
	if err == nil {
		t.Fatal("openMQTTConnection() returned no TCP error")
	}
}

func TestOpenMQTTConnectionEstablishesActiveMQTLS(t *testing.T) {
	server := httptest.NewTLSServer(nil)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	brokerURL, err := url.Parse("mqtt+ssl://" + serverURL.Host)
	if err != nil {
		t.Fatal(err)
	}
	options := *mqtt.NewClientOptions().
		SetConnectTimeout(time.Second).
		SetTLSConfig(&tls.Config{InsecureSkipVerify: true}) // #nosec G402 -- local test server

	connection, err := openMQTTConnection(brokerURL, options)
	if err != nil {
		t.Fatalf("openMQTTConnection() error = %v", err)
	}
	defer connection.Close()
	if _, ok := connection.(*tls.Conn); !ok {
		t.Fatalf("openMQTTConnection() connection type = %T, want *tls.Conn", connection)
	}
}

func TestIsSecureMQTTScheme(t *testing.T) {
	for _, scheme := range []string{"ssl", "tls", "mqtts", "mqtt+ssl", "tcps"} {
		if !isSecureMQTTScheme(scheme) {
			t.Errorf("isSecureMQTTScheme(%q) = false, want true", scheme)
		}
	}
	if isSecureMQTTScheme("tcp") {
		t.Error("isSecureMQTTScheme(\"tcp\") = true, want false")
	}
}

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

func TestParseMQTTBrokers(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "single", raw: "mqtt+ssl://broker.example:8883", want: []string{"mqtt+ssl://broker.example:8883"}},
		{name: "empty", raw: "  ", want: []string{}},
		{
			name: "Amazon MQ failover",
			raw:  "failover:(mqtt+ssl://b-1.mq.eu-west-2.amazonaws.com:8883,mqtt+ssl://b-2.mq.eu-west-2.amazonaws.com:8883)",
			want: []string{"mqtt+ssl://b-1.mq.eu-west-2.amazonaws.com:8883", "mqtt+ssl://b-2.mq.eu-west-2.amazonaws.com:8883"},
		},
		{
			name: "failover with spaces and transport options",
			raw:  " FAILOVER:( ssl://b-1.example:8883 , ssl://b-2.example:8883 )?randomize=false",
			want: []string{"ssl://b-1.example:8883", "ssl://b-2.example:8883"},
		},
		{name: "failover without parentheses", raw: "failover:tcp://b-1.example:1883", want: []string{"tcp://b-1.example:1883"}},
		{name: "comma separated", raw: "tcp://b-1.example:1883,tcp://b-2.example:1883", want: []string{"tcp://b-1.example:1883", "tcp://b-2.example:1883"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := parseMQTTBrokers(test.raw)
			if len(got) != len(test.want) {
				t.Fatalf("parseMQTTBrokers(%q) = %q, want %q", test.raw, got, test.want)
			}
			for i := range got {
				if got[i] != test.want[i] {
					t.Fatalf("parseMQTTBrokers(%q) = %q, want %q", test.raw, got, test.want)
				}
			}
		})
	}
}

func TestConfigureMQTTOptionsRegistersFailoverBrokers(t *testing.T) {
	options := mqtt.NewClientOptions()
	for _, broker := range parseMQTTBrokers("failover:(mqtt+ssl://b-1.example:8883,mqtt+ssl://b-2.example:8883)") {
		options.AddBroker(broker)
	}
	if len(options.Servers) != 2 {
		t.Fatalf("registered %d brokers, want 2", len(options.Servers))
	}
	for i, host := range []string{"b-1.example:8883", "b-2.example:8883"} {
		if options.Servers[i].Scheme != "mqtt+ssl" || options.Servers[i].Host != host {
			t.Fatalf("broker %d = %s, want mqtt+ssl://%s", i, options.Servers[i], host)
		}
	}
}
