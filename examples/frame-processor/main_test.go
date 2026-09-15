package main

import "testing"

func TestLoadConfigRejectsInvalidBounds(t *testing.T) {
	t.Setenv("FRAME_PROCESSOR_API_TOKEN", "secret")
	t.Setenv("FRAME_PROCESSOR_MQTT_URI", "tcp://localhost:1883")
	t.Setenv("FRAME_PROCESSOR_HUB_KEY", "hub")
	t.Setenv("FRAME_PROCESSOR_BRIGHTNESS_THRESHOLD", "256")

	if _, err := loadConfig(); err == nil {
		t.Fatal("loadConfig() accepted an invalid brightness threshold")
	}
}

func TestLoadConfigRejectsUnknownProfile(t *testing.T) {
	t.Setenv("FRAME_PROCESSOR_API_TOKEN", "secret")
	t.Setenv("FRAME_PROCESSOR_MQTT_URI", "tcp://localhost:1883")
	t.Setenv("FRAME_PROCESSOR_HUB_KEY", "hub")
	t.Setenv("FRAME_PROCESSOR_PROFILE", "unknown")

	if _, err := loadConfig(); err == nil {
		t.Fatal("loadConfig() accepted an unknown profile")
	}
}
