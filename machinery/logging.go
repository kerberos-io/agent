package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	defaultLogLevel  = log.InfoLevel
	defaultLogOutput = "text"
)

type localTimeFormatter struct {
	timezone  *time.Location
	formatter log.Formatter
}

func (f localTimeFormatter) Format(entry *log.Entry) ([]byte, error) {
	entry.Time = entry.Time.In(f.timezone)
	return f.formatter.Format(entry)
}

type componentHook struct{}

func (componentHook) Levels() []log.Level {
	return log.AllLevels
}

func (componentHook) Fire(entry *log.Entry) error {
	if _, exists := entry.Data["component"]; exists {
		return nil
	}
	entry.Data["component"] = componentFromCaller(entry.Caller)
	return nil
}

func configureLogging(levelValue string, outputValue string, timezone *time.Location) {
	configureLogger(log.StandardLogger(), levelValue, outputValue, timezone)
}

func configureLogger(logger *log.Logger, levelValue string, outputValue string, timezone *time.Location) {
	if timezone == nil {
		timezone = time.Local
	}

	level, levelErr := parseLogLevel(levelValue)
	output, outputErr := parseLogOutput(outputValue)

	logger.SetOutput(os.Stdout)
	logger.SetLevel(level)
	logger.SetReportCaller(true)
	logger.SetFormatter(localTimeFormatter{
		timezone:  timezone,
		formatter: newLogFormatter(output),
	})
	installComponentHook(logger)

	if levelErr != nil {
		logger.WithFields(log.Fields{
			"configured_level": levelValue,
			"effective_level":  level.String(),
		}).WithError(levelErr).Warn("invalid log level; using default")
	}
	if outputErr != nil {
		logger.WithFields(log.Fields{
			"configured_output": outputValue,
			"effective_output":  output,
		}).WithError(outputErr).Warn("invalid log output; using default")
	}

	logger.WithFields(log.Fields{
		"event":         "logger_configured",
		"log_level":     level.String(),
		"output":        output,
		"report_caller": logger.ReportCaller,
		"timezone":      timezone.String(),
	}).Debug("logging configured")
}

func installComponentHook(logger *log.Logger) {
	for _, hooks := range logger.Hooks {
		for _, hook := range hooks {
			if _, ok := hook.(componentHook); ok {
				return
			}
		}
	}
	logger.AddHook(componentHook{})
}

func componentFromCaller(frame *runtime.Frame) string {
	if frame == nil {
		return "unknown"
	}

	const sourceMarker = "/machinery/src/"
	normalizedFile := filepath.ToSlash(frame.File)
	if markerIndex := strings.Index(normalizedFile, sourceMarker); markerIndex >= 0 {
		relativeFile := normalizedFile[markerIndex+len(sourceMarker):]
		if directory := filepath.ToSlash(filepath.Dir(relativeFile)); directory != "." {
			return directory
		}
	}
	if strings.Contains(normalizedFile, "/machinery/") {
		return "agent"
	}
	return "unknown"
}

func parseLogLevel(value string) (log.Level, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return defaultLogLevel, nil
	}
	if normalized == "warning" {
		normalized = "warn"
	}

	level, err := log.ParseLevel(normalized)
	if err != nil {
		return defaultLogLevel, fmt.Errorf("parse LOG_LEVEL: %w", err)
	}
	return level, nil
}

func parseLogOutput(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return defaultLogOutput, nil
	}
	switch normalized {
	case "json", "text":
		return normalized, nil
	default:
		return defaultLogOutput, fmt.Errorf("unsupported LOG_OUTPUT %q", value)
	}
}

func newLogFormatter(output string) log.Formatter {
	callerPrettyfier := func(frame *runtime.Frame) (string, string) {
		return filepath.Base(frame.Function), fmt.Sprintf("%s:%d", filepath.Base(frame.File), frame.Line)
	}
	if output == "json" {
		return &log.JSONFormatter{
			CallerPrettyfier: callerPrettyfier,
			TimestampFormat:  time.RFC3339Nano,
		}
	}
	return &log.TextFormatter{
		CallerPrettyfier: callerPrettyfier,
		FullTimestamp:    true,
		TimestampFormat:  time.RFC3339Nano,
	}
}
