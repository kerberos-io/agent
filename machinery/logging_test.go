package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
)

func TestConfigureLoggingDebugIncludesStructuredContext(t *testing.T) {
	logger := log.New()
	var output bytes.Buffer
	configureLogger(logger, &output, "debug", "json", time.UTC)
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
	configureLogger(logger, io.Discard, "info", "text", time.UTC)
	configureLogger(logger, io.Discard, "debug", "json", time.UTC)

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

func discardLogger() *log.Logger {
	logger := log.New()
	logger.SetOutput(io.Discard)
	return logger
}

func TestOpenLogFileDisabledByDefault(t *testing.T) {
	t.Setenv("LOG_FILE", "")
	configDirectory := t.TempDir()

	if logFile := openLogFile(discardLogger(), configDirectory); logFile != nil {
		logFile.Close()
		t.Fatalf("openLogFile() = %q, want nil when LOG_FILE is unset", logFile.Filename)
	}
	if _, err := os.Stat(filepath.Join(configDirectory, defaultLogDirectory)); !os.IsNotExist(err) {
		t.Fatalf("log directory was created while file logging is disabled: %v", err)
	}
}

func TestOpenLogFileWritesEntriesToStdoutAndFile(t *testing.T) {
	t.Setenv("LOG_FILE", "true")
	t.Setenv("LOG_DIRECTORY", "")
	t.Setenv("LOG_FILE_MAX_SIZE", "")
	t.Setenv("LOG_FILE_MAX_BACKUPS", "")
	t.Setenv("LOG_FILE_MAX_AGE", "")
	configDirectory := t.TempDir()

	logFile := openLogFile(discardLogger(), configDirectory)
	if logFile == nil {
		t.Fatal("openLogFile() = nil, want a log file when LOG_FILE=true")
	}
	t.Cleanup(func() { logFile.Close() })

	wantFilename := filepath.Join(configDirectory, "data", "log", "agent.log")
	if logFile.Filename != wantFilename {
		t.Fatalf("Filename = %q, want %q", logFile.Filename, wantFilename)
	}
	if logFile.MaxSize != defaultLogFileMaxSize || logFile.MaxBackups != defaultLogFileMaxBackups || logFile.MaxAge != defaultLogFileMaxAge {
		t.Fatalf("limits = (size %d, backups %d, age %d), want defaults (%d, %d, %d)",
			logFile.MaxSize, logFile.MaxBackups, logFile.MaxAge,
			defaultLogFileMaxSize, defaultLogFileMaxBackups, defaultLogFileMaxAge)
	}
	info, err := os.Stat(wantFilename)
	if err != nil {
		t.Fatalf("log file was not created at startup: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("log file mode = %o, want 600", mode)
	}

	logger := log.New()
	var stdout bytes.Buffer
	configureLogger(logger, io.MultiWriter(&stdout, logFile), "info", "json", time.UTC)
	logger.WithField("event", "test_event").Info("written to stdout and file")

	contents, err := os.ReadFile(wantFilename)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	for name, output := range map[string]string{"stdout": stdout.String(), "log file": string(contents)} {
		if !strings.Contains(output, `"msg":"written to stdout and file"`) {
			t.Fatalf("%s is missing the log entry: %q", name, output)
		}
	}
}

func TestOpenLogFileUsesConfiguredDirectoryAndLimits(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "logs")
	t.Setenv("LOG_FILE", "TRUE")
	t.Setenv("LOG_DIRECTORY", directory)
	t.Setenv("LOG_FILE_MAX_SIZE", "25")
	t.Setenv("LOG_FILE_MAX_BACKUPS", "0")
	t.Setenv("LOG_FILE_MAX_AGE", " 7 ")

	logFile := openLogFile(discardLogger(), t.TempDir())
	if logFile == nil {
		t.Fatal("openLogFile() = nil, want a log file when LOG_FILE=TRUE")
	}
	t.Cleanup(func() { logFile.Close() })

	if want := filepath.Join(directory, "agent.log"); logFile.Filename != want {
		t.Fatalf("Filename = %q, want %q", logFile.Filename, want)
	}
	if logFile.MaxSize != 25 || logFile.MaxBackups != 0 || logFile.MaxAge != 7 {
		t.Fatalf("limits = (size %d, backups %d, age %d), want (25, 0, 7)",
			logFile.MaxSize, logFile.MaxBackups, logFile.MaxAge)
	}
}

func TestOpenLogFileInvalidLimitUsesDefault(t *testing.T) {
	t.Setenv("LOG_FILE", "true")
	t.Setenv("LOG_DIRECTORY", t.TempDir())
	t.Setenv("LOG_FILE_MAX_SIZE", "0")
	t.Setenv("LOG_FILE_MAX_BACKUPS", "")
	t.Setenv("LOG_FILE_MAX_AGE", "")

	logger := log.New()
	var output bytes.Buffer
	configureLogger(logger, &output, "info", "json", time.UTC)

	logFile := openLogFile(logger, t.TempDir())
	if logFile == nil {
		t.Fatal("openLogFile() = nil, want a log file when LOG_FILE=true")
	}
	t.Cleanup(func() { logFile.Close() })

	if logFile.MaxSize != defaultLogFileMaxSize {
		t.Fatalf("MaxSize = %d, want default %d", logFile.MaxSize, defaultLogFileMaxSize)
	}
	if !strings.Contains(output.String(), `"setting":"LOG_FILE_MAX_SIZE"`) {
		t.Fatalf("missing warning for invalid LOG_FILE_MAX_SIZE: %q", output.String())
	}
}

func TestOpenLogFileUnwritableDirectoryKeepsStdout(t *testing.T) {
	// A directory below a regular file can never be created, also not as root.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatalf("create blocker file: %v", err)
	}
	t.Setenv("LOG_FILE", "true")
	t.Setenv("LOG_DIRECTORY", filepath.Join(blocker, "log"))

	logger := log.New()
	var output bytes.Buffer
	configureLogger(logger, &output, "info", "json", time.UTC)

	if logFile := openLogFile(logger, t.TempDir()); logFile != nil {
		logFile.Close()
		t.Fatalf("openLogFile() = %q, want nil for an unwritable directory", logFile.Filename)
	}
	if !strings.Contains(output.String(), `"event":"log_file_unavailable"`) {
		t.Fatalf("missing log_file_unavailable warning: %q", output.String())
	}
}

func TestParseLogFileEnabled(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    bool
		wantErr bool
	}{
		{name: "default", want: false},
		{name: "true", value: "true", want: true},
		{name: "uppercase", value: "TRUE", want: true},
		{name: "numeric", value: "1", want: true},
		{name: "false", value: "false", want: false},
		{name: "trimmed", value: " true ", want: true},
		{name: "invalid", value: "yes", want: false, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseLogFileEnabled(test.value)
			if (err != nil) != test.wantErr {
				t.Fatalf("parseLogFileEnabled(%q) error = %v, wantErr %t", test.value, err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("parseLogFileEnabled(%q) = %t, want %t", test.value, got, test.want)
			}
		})
	}
}

func TestParseLogFileLimit(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		minimum int
		want    int
		wantErr bool
	}{
		{name: "default", minimum: 1, want: 10},
		{name: "value", value: "25", minimum: 1, want: 25},
		{name: "trimmed", value: " 3 ", minimum: 0, want: 3},
		{name: "zero allowed", value: "0", minimum: 0, want: 0},
		{name: "below minimum", value: "0", minimum: 1, want: 10, wantErr: true},
		{name: "negative", value: "-1", minimum: 0, want: 10, wantErr: true},
		{name: "invalid", value: "10MB", minimum: 1, want: 10, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseLogFileLimit("LOG_FILE_MAX_SIZE", test.value, 10, test.minimum)
			if (err != nil) != test.wantErr {
				t.Fatalf("parseLogFileLimit(%q) error = %v, wantErr %t", test.value, err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("parseLogFileLimit(%q) = %d, want %d", test.value, got, test.want)
			}
		})
	}
}
