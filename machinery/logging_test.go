package main

import (
	"bytes"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
)

func TestConfigureLoggingDebugIncludesStructuredContext(t *testing.T) {
	logger := log.New()
	var output bytes.Buffer
	configureLogger(logger, "debug", "json", time.UTC)
	logger.SetOutput(&output)
	logger.WithField("event", "test_event").Debug("structured debug test")

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &entry); err != nil {
		t.Fatalf("decode debug log: %v; output=%q", err, output.String())
	}
	for key, want := range map[string]interface{}{
		"component": "agent",
		"event":     "test_event",
		"level":     "debug",
		"msg":       "structured debug test",
	} {
		if got := entry[key]; got != want {
			t.Fatalf("%s = %v, want %v", key, got, want)
		}
	}
	if entry["file"] == nil || entry["func"] == nil {
		t.Fatalf("debug log is missing caller metadata: %v", entry)
	}
}

func TestConfigureLoggerInstallsComponentHookOnce(t *testing.T) {
	logger := log.New()
	configureLogger(logger, "info", "text", time.UTC)
	configureLogger(logger, "debug", "json", time.UTC)

	var componentHooks int
	for _, hooks := range logger.Hooks {
		for _, hook := range hooks {
			if _, ok := hook.(componentHook); ok {
				componentHooks++
			}
		}
	}
	if componentHooks != len(log.AllLevels) {
		t.Fatalf("component hook registrations = %d, want %d", componentHooks, len(log.AllLevels))
	}
}

func TestComponentFromCaller(t *testing.T) {
	tests := []struct {
		name  string
		frame *runtime.Frame
		want  string
	}{
		{
			name:  "nested runtime package",
			frame: &runtime.Frame{File: "/workspace/agent/machinery/src/routers/mqtt/main.go"},
			want:  "routers/mqtt",
		},
		{
			name:  "top-level runtime package",
			frame: &runtime.Frame{File: "/workspace/agent/machinery/src/capture/main.go"},
			want:  "capture",
		},
		{
			name:  "executable",
			frame: &runtime.Frame{File: "/workspace/agent/machinery/main.go"},
			want:  "agent",
		},
		{name: "missing caller", want: "unknown"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := componentFromCaller(test.frame); got != test.want {
				t.Fatalf("componentFromCaller() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestComponentHookAddsAndPreservesComponent(t *testing.T) {
	hook := componentHook{}

	entry := log.NewEntry(log.New())
	entry.Caller = &runtime.Frame{File: "/workspace/agent/machinery/src/cloud/livehls/session.go"}
	if err := hook.Fire(entry); err != nil {
		t.Fatalf("componentHook.Fire() error = %v", err)
	}
	if got := entry.Data["component"]; got != "cloud/livehls" {
		t.Fatalf("component = %v, want cloud/livehls", got)
	}

	entry.Data["component"] = "explicit"
	if err := hook.Fire(entry); err != nil {
		t.Fatalf("componentHook.Fire() preserving field error = %v", err)
	}
	if got := entry.Data["component"]; got != "explicit" {
		t.Fatalf("component = %v, want explicit", got)
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    log.Level
		wantErr bool
	}{
		{name: "default", want: log.InfoLevel},
		{name: "info", value: "INFO", want: log.InfoLevel},
		{name: "warning alias", value: "warning", want: log.WarnLevel},
		{name: "debug", value: "debug", want: log.DebugLevel},
		{name: "trace", value: "trace", want: log.TraceLevel},
		{name: "invalid", value: "verbose", want: log.InfoLevel, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseLogLevel(test.value)
			if (err != nil) != test.wantErr {
				t.Fatalf("parseLogLevel(%q) error = %v, wantErr %t", test.value, err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("parseLogLevel(%q) = %s, want %s", test.value, got, test.want)
			}
		})
	}
}

func TestParseLogOutput(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{name: "default", want: "text"},
		{name: "text", value: "TEXT", want: "text"},
		{name: "json", value: "json", want: "json"},
		{name: "invalid", value: "console", want: "text", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseLogOutput(test.value)
			if (err != nil) != test.wantErr {
				t.Fatalf("parseLogOutput(%q) error = %v, wantErr %t", test.value, err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("parseLogOutput(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}
