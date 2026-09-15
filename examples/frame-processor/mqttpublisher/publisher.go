package mqttpublisher

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/kerberos-io/agent/examples/frame-processor/contract"
)

const (
	commandQoS = byte(1)
	statusQoS  = byte(1)
)

type Config struct {
	BrokerURI string
	Username  string
	Password  string
	HubKey    string
	ClientID  string
	Timeout   time.Duration
}

type StatusHandler func(contract.StatusEvent)

type Publisher struct {
	client       mqtt.Client
	commandTopic string
	timeout      time.Duration
}

func New(config Config, handler StatusHandler) (*Publisher, error) {
	if config.BrokerURI == "" || config.HubKey == "" {
		return nil, errors.New("MQTT broker URI and Hub key are required")
	}
	if config.Timeout <= 0 {
		config.Timeout = 10 * time.Second
	}
	if config.ClientID == "" {
		config.ClientID = "frame-processor-" + randomID()
	}

	statusTopic := "kerberos/hub/" + config.HubKey
	options := mqtt.NewClientOptions().
		AddBroker(config.BrokerURI).
		SetClientID(config.ClientID).
		SetUsername(config.Username).
		SetPassword(config.Password).
		SetCleanSession(false).
		SetResumeSubs(true).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetMaxReconnectInterval(time.Minute).
		SetKeepAlive(30 * time.Second).
		SetPingTimeout(10 * time.Second)
	if handler != nil {
		options.SetOnConnectHandler(func(client mqtt.Client) {
			token := client.Subscribe(statusTopic, statusQoS, statusMessageHandler(handler))
			if !token.WaitTimeout(config.Timeout) || token.Error() != nil {
				slog.Error("failed to subscribe to Agent status events", "topic", statusTopic, "error", token.Error())
			}
		})
	}

	client := mqtt.NewClient(options)
	token := client.Connect()
	if !token.WaitTimeout(config.Timeout) {
		return nil, errors.New("MQTT connection timed out")
	}
	if err := token.Error(); err != nil {
		return nil, fmt.Errorf("connect MQTT: %w", err)
	}
	return &Publisher{
		client: client, commandTopic: "kerberos/agent/" + config.HubKey,
		timeout: config.Timeout,
	}, nil
}

func (p *Publisher) Close() {
	if p != nil && p.client != nil && p.client.IsConnected() {
		p.client.Disconnect(250)
	}
}

func (p *Publisher) PublishCaptureFrame(ctx context.Context, deviceID string, command contract.CaptureFrameCommand) error {
	return p.publish(ctx, deviceID, contract.ActionCaptureFrame, command)
}

func (p *Publisher) PublishRecordingWindow(ctx context.Context, deviceID string, command contract.RecordingWindowCommand) error {
	return p.publish(ctx, deviceID, contract.ActionRequestRecordingWindow, command)
}

func (p *Publisher) publish(ctx context.Context, deviceID, action string, value any) error {
	message := newMessage(deviceID, action, value, time.Now())
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal MQTT command: %w", err)
	}
	token := p.client.Publish(p.commandTopic, commandQoS, false, payload)
	timer := time.NewTimer(p.timeout)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return errors.New("MQTT publish timed out")
	case <-token.Done():
		if err := token.Error(); err != nil {
			return fmt.Errorf("publish MQTT command: %w", err)
		}
		return nil
	}
}

func newMessage(deviceID, action string, value any, now time.Time) contract.MQTTMessage {
	return contract.MQTTMessage{
		MID:       randomID(),
		DeviceID:  deviceID,
		Timestamp: now.Unix(),
		Payload: contract.MQTTPayload{
			Version:  contract.SchemaVersion,
			Action:   action,
			DeviceID: deviceID,
			Value:    value,
		},
	}
}

func randomID() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value)
	return strings.Join([]string{encoded[0:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:32]}, "-")
}

func statusMessageHandler(handler StatusHandler) mqtt.MessageHandler {
	return func(_ mqtt.Client, message mqtt.Message) {
		var envelope contract.MQTTMessage
		if err := json.Unmarshal(message.Payload(), &envelope); err != nil {
			slog.Warn("discarding malformed Agent status envelope", "error", err)
			return
		}
		if envelope.Payload.Action != contract.ActionFrameStatus {
			return
		}
		value, err := json.Marshal(envelope.Payload.Value)
		if err != nil {
			slog.Warn("discarding unencodable Agent status value", "error", err)
			return
		}
		var status contract.StatusEvent
		if err := json.Unmarshal(value, &status); err != nil {
			slog.Warn("discarding malformed Agent status value", "error", err)
			return
		}
		handler(status)
	}
}
