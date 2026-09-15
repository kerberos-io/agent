package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/kerberos-io/agent/examples/frame-processor/contract"
	"github.com/kerberos-io/agent/examples/frame-processor/mqttpublisher"
	"github.com/kerberos-io/agent/examples/frame-processor/processor"
	"github.com/kerberos-io/agent/examples/frame-processor/service"
)

func main() {
	config, err := loadConfig()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	publisher, err := mqttpublisher.New(config.mqtt, func(status contract.StatusEvent) {
		slog.Info("Agent frame-processing status",
			"deviceId", status.DeviceID,
			"requestId", status.RequestID,
			"frameId", status.FrameID,
			"status", status.Status,
		)
	})
	if err != nil {
		slog.Error("failed to initialize MQTT", "error", err)
		os.Exit(1)
	}
	defer publisher.Close()

	engine := processor.New(config.processor)
	application := service.New(config.service, engine, publisher, nil)
	server := &http.Server{
		Addr:              config.address,
		Handler:           application.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownContext)
	}()

	slog.Info("Frame Processor listening", "address", config.address, "profile", config.processor.DefaultProfile)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("Frame Processor stopped", "error", err)
		os.Exit(1)
	}
}

type applicationConfig struct {
	address   string
	mqtt      mqttpublisher.Config
	processor processor.Config
	service   service.Config
}

func loadConfig() (applicationConfig, error) {
	brightnessThreshold := envInt("FRAME_PROCESSOR_BRIGHTNESS_THRESHOLD", 200)
	config := applicationConfig{
		address: envString("FRAME_PROCESSOR_ADDRESS", ":8080"),
		mqtt: mqttpublisher.Config{
			BrokerURI: os.Getenv("FRAME_PROCESSOR_MQTT_URI"),
			Username:  os.Getenv("FRAME_PROCESSOR_MQTT_USERNAME"),
			Password:  os.Getenv("FRAME_PROCESSOR_MQTT_PASSWORD"),
			HubKey:    os.Getenv("FRAME_PROCESSOR_HUB_KEY"),
			ClientID:  os.Getenv("FRAME_PROCESSOR_MQTT_CLIENT_ID"),
			Timeout:   envDurationSeconds("FRAME_PROCESSOR_MQTT_TIMEOUT_SECONDS", 10),
		},
		processor: processor.Config{
			DefaultProfile:      envString("FRAME_PROCESSOR_PROFILE", processor.ProfileNeverTrigger),
			EveryN:              envInt("FRAME_PROCESSOR_EVERY_N", 2),
			BrightnessThreshold: uint8(brightnessThreshold),
			Delay:               envDurationMillis("FRAME_PROCESSOR_DELAY_MILLISECONDS", 0),
			ForceError:          envBool("FRAME_PROCESSOR_FORCE_ERROR", false),
		},
		service: service.Config{
			APIToken:         os.Getenv("FRAME_PROCESSOR_API_TOKEN"),
			MaxFrameBytes:    int64(envInt("FRAME_PROCESSOR_MAX_FRAME_BYTES", 4<<20)),
			MaxFrameTTL:      envDurationSeconds("FRAME_PROCESSOR_MAX_FRAME_TTL_SECONDS", 300),
			CommandTTL:       envDurationSeconds("FRAME_PROCESSOR_COMMAND_TTL_SECONDS", 30),
			PreRollSeconds:   int64(envInt("FRAME_PROCESSOR_PRE_ROLL_SECONDS", 10)),
			EventClipSeconds: int64(envInt("FRAME_PROCESSOR_EVENT_CLIP_SECONDS", 30)),
		},
	}
	if config.service.APIToken == "" {
		return applicationConfig{}, errors.New("FRAME_PROCESSOR_API_TOKEN is required")
	}
	if config.mqtt.BrokerURI == "" || config.mqtt.HubKey == "" {
		return applicationConfig{}, errors.New("FRAME_PROCESSOR_MQTT_URI and FRAME_PROCESSOR_HUB_KEY are required")
	}
	if config.processor.EveryN <= 0 {
		return applicationConfig{}, errors.New("FRAME_PROCESSOR_EVERY_N must be positive")
	}
	if !processor.IsProfileSupported(config.processor.DefaultProfile) {
		return applicationConfig{}, fmt.Errorf("unsupported FRAME_PROCESSOR_PROFILE %q", config.processor.DefaultProfile)
	}
	if brightnessThreshold < 0 || brightnessThreshold > 255 {
		return applicationConfig{}, errors.New("FRAME_PROCESSOR_BRIGHTNESS_THRESHOLD must be between 0 and 255")
	}
	if config.processor.Delay < 0 || config.processor.Delay > time.Minute {
		return applicationConfig{}, errors.New("FRAME_PROCESSOR_DELAY_MILLISECONDS must be between 0 and 60000")
	}
	if config.service.MaxFrameBytes <= 0 || config.service.MaxFrameBytes > 100<<20 {
		return applicationConfig{}, errors.New("FRAME_PROCESSOR_MAX_FRAME_BYTES must be between 1 and 104857600")
	}
	if config.service.MaxFrameTTL <= 0 || config.service.MaxFrameTTL > time.Hour {
		return applicationConfig{}, errors.New("FRAME_PROCESSOR_MAX_FRAME_TTL_SECONDS must be between 1 and 3600")
	}
	if config.service.PreRollSeconds < 0 || config.service.PreRollSeconds > config.service.EventClipSeconds {
		return applicationConfig{}, errors.New("FRAME_PROCESSOR_PRE_ROLL_SECONDS must be between zero and FRAME_PROCESSOR_EVENT_CLIP_SECONDS")
	}
	if config.service.CommandTTL <= 0 || config.service.CommandTTL > time.Hour {
		return applicationConfig{}, errors.New("FRAME_PROCESSOR_COMMAND_TTL_SECONDS must be between 1 and 3600")
	}
	if config.service.EventClipSeconds <= 0 || config.service.EventClipSeconds > 24*60*60 {
		return applicationConfig{}, errors.New("FRAME_PROCESSOR_EVENT_CLIP_SECONDS must be between 1 and 86400")
	}
	if config.mqtt.Timeout <= 0 || config.mqtt.Timeout > time.Minute {
		return applicationConfig{}, errors.New("FRAME_PROCESSOR_MQTT_TIMEOUT_SECONDS must be between 1 and 60")
	}
	return config, nil
}

func envString(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envInt(name string, fallback int) int {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		slog.Warn("invalid integer environment value, using default", "name", name)
		return fallback
	}
	return parsed
}

func envBool(name string, fallback bool) bool {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		slog.Warn("invalid boolean environment value, using default", "name", name)
		return fallback
	}
	return parsed
}

func envDurationSeconds(name string, fallback int) time.Duration {
	return time.Duration(envInt(name, fallback)) * time.Second
}

func envDurationMillis(name string, fallback int) time.Duration {
	return time.Duration(envInt(name, fallback)) * time.Millisecond
}

func (c applicationConfig) String() string {
	return fmt.Sprintf("address=%s profile=%s", c.address, c.processor.DefaultProfile)
}
