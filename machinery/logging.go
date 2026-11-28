package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	defaultLogLevel  = log.InfoLevel
	defaultLogOutput = "text"

	// Log files are written when LOG_FILE is enabled. The default directory
	// is relative to the configuration directory and is created by the
	// Docker image.
	logFileName              = "agent.log"
	defaultLogDirectory      = "data/log"
	defaultLogFileMaxSize    = 10 // megabytes
	defaultLogFileMaxBackups = 5
	defaultLogFileMaxAge     = 0 // days; 0 keeps rotated files regardless of age
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

func configureLogging(writer io.Writer, levelValue string, outputValue string, timezone *time.Location) {
	configureLogger(log.StandardLogger(), writer, levelValue, outputValue, timezone)
}

func configureLogger(logger *log.Logger, writer io.Writer, levelValue string, outputValue string, timezone *time.Location) {
	if timezone == nil {
		timezone = time.Local
	}

	level, levelErr := parseLogLevel(levelValue)
	output, outputErr := parseLogOutput(outputValue)

	logger.SetOutput(writer)
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

// openLogFile returns a size-rotated log file when LOG_FILE is enabled, and nil
// otherwise. The file is named agent.log and lives in LOG_DIRECTORY, which
// defaults to data/log inside the configuration directory. Invalid limits fall
// back to their defaults, and a log file that cannot be written leaves logging
// on stdout only. Both are reported through logger instead of stopping the
// Agent.
func openLogFile(logger *log.Logger, configDirectory string) *lumberjack.Logger {
	enabledValue := os.Getenv("LOG_FILE")
	enabled, err := parseLogFileEnabled(enabledValue)
	if err != nil {
		logger.WithFields(log.Fields{
			"configured_value": enabledValue,
			"setting":          "LOG_FILE",
		}).WithError(err).Warn("invalid LOG_FILE; writing logs to stdout only")
	}
	if !enabled {
		return nil
	}

	limit := func(name string, defaultValue int, minimum int) int {
		value := os.Getenv(name)
		parsed, err := parseLogFileLimit(name, value, defaultValue, minimum)
		if err != nil {
			logger.WithFields(log.Fields{
				"configured_value": value,
				"effective_value":  parsed,
				"setting":          name,
			}).WithError(err).Warn("invalid log file setting; using default")
		}
		return parsed
	}

	directory := strings.TrimSpace(os.Getenv("LOG_DIRECTORY"))
	if directory == "" {
		directory = filepath.Join(configDirectory, defaultLogDirectory)
	}
	logFile := &lumberjack.Logger{
		Filename:   filepath.Join(directory, logFileName),
		MaxSize:    limit("LOG_FILE_MAX_SIZE", defaultLogFileMaxSize, 1),
		MaxBackups: limit("LOG_FILE_MAX_BACKUPS", defaultLogFileMaxBackups, 0),
		MaxAge:     limit("LOG_FILE_MAX_AGE", defaultLogFileMaxAge, 0),
	}

	if err := checkLogFileWritable(logFile.Filename); err != nil {
		logger.WithFields(log.Fields{
			"event":    "log_file_unavailable",
			"log_file": logFile.Filename,
		}).WithError(err).Warn("cannot write log file; writing logs to stdout only")
		return nil
	}

	logger.WithFields(log.Fields{
		"event":        "log_file_configured",
		"log_file":     logFile.Filename,
		"max_size_mb":  logFile.MaxSize,
		"max_backups":  logFile.MaxBackups,
		"max_age_days": logFile.MaxAge,
	}).Info("writing logs to rotating files")
	return logFile
}

// checkLogFileWritable creates the log directory and file when they do not
// exist yet, so a location the Agent cannot write to is reported once at
// startup instead of failing every log entry.
func checkLogFileWritable(filename string) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	return file.Close()
}

func parseLogFileEnabled(value string) (bool, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return false, nil
	}

	enabled, err := strconv.ParseBool(normalized)
	if err != nil {
		return false, fmt.Errorf("parse LOG_FILE: %w", err)
	}
	return enabled, nil
}

func parseLogFileLimit(name string, value string, defaultValue int, minimum int) (int, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return defaultValue, nil
	}

	limit, err := strconv.Atoi(normalized)
	if err != nil {
		return defaultValue, fmt.Errorf("parse %s: %w", name, err)
	}
	if limit < minimum {
		return defaultValue, fmt.Errorf("%s must be at least %d, got %d", name, minimum, limit)
	}
	return limit, nil
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
