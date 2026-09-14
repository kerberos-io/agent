package mqtt

import (
	"crypto/tls"
	"net/http/httptest"
	"net/url"
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := mqtt.NewClientOptions()
			enableMQTTConnectionDiagnostics(options, test.brokerURL)
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
