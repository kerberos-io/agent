package log

import (
	"os"
	"time"

	"github.com/op/go-logging"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

// The logging library being used everywhere.
var Log = Logging{
	Logger: "logrus",
}

// -----------------
// This a gologging
// -> github.com/op/go-logging

var gologging = logging.MustGetLogger("gologger")

func ConfigureGoLogging(configDirectory string, timezone *time.Location) {
	// Logging
	var format = logging.MustStringFormatter(
		`%{color}%{time:15:04:05.000} %{shortfunc} ▶ %{level:.4s} %{id:03x}%{color:reset} %{message}`,
	)
	var format2 = logging.MustStringFormatter(
		`%{time:15:04:05.000} %{shortfunc} ▶ %{level:.4s}  %{id:03x}%{color:reset} %{message}`,
	)
	stdBackend := logging.NewLogBackend(os.Stderr, "", 0)
	stdBackendLeveled := logging.NewBackendFormatter(stdBackend, format)
	fileBackend := logging.NewLogBackend(&lumberjack.Logger{
		Filename: configDirectory + "/data/log/machinery.txt",
		MaxSize:  2,    // megabytes
		Compress: true, // disabled by default
	}, "", 0)
	fileBackendLeveled := logging.NewBackendFormatter(fileBackend, format2)
	logging.SetBackend(stdBackendLeveled, fileBackendLeveled)
	logging.SetLevel(logging.DEBUG, "")
}

// -----------------
// This a logrus
// -> github.com/sirupsen/logrus

func ConfigureLogrus(level string, timezone *time.Location) {
	// Log as JSON instead of the default ASCII formatter.
	logrus.SetFormatter(LocalTimeZoneFormatter{
		Timezone:  timezone,
		Formatter: &logrus.JSONFormatter{},
	}) // Use local timezone for providing datetime in logs!

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	logrus.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	logLevel := logrus.InfoLevel
	if level == "error" {
		logLevel = logrus.ErrorLevel
	} else if level == "debug" {
		logLevel = logrus.DebugLevel
	} else if level == "fatal" {
		logLevel = logrus.FatalLevel
	} else if level == "warning" {
		logLevel = logrus.WarnLevel
	}
	logrus.SetLevel(logLevel)
}

type LocalTimeZoneFormatter struct {
	Timezone  *time.Location
	Formatter logrus.Formatter
}

func (u LocalTimeZoneFormatter) Format(e *logrus.Entry) ([]byte, error) {
	e.Time = e.Time.In(u.Timezone)
	return u.Formatter.Format(e)
}

type Logging struct {
	Logger string
}

func (self *Logging) Init(level string, configDirectory string, timezone *time.Location) {
	switch self.Logger {
	case "go-logging":
		ConfigureGoLogging(configDirectory, timezone)
	case "logrus":
		ConfigureLogrus(level, timezone)
	default:
	}
}

func (self *Logging) Info(sentence string) {
	switch self.Logger {
	case "go-logging":
		gologging.Info(sentence)
	case "logrus":
		logrus.Info(sentence)
	default:
	}
}

func (self *Logging) Warning(sentence string) {
	switch self.Logger {
	case "go-logging":
		gologging.Warning(sentence)
	case "logrus":
		logrus.Warn(sentence)
	default:
	}
}

func (self *Logging) Debug(sentence string) {
	switch self.Logger {
	case "go-logging":
		gologging.Debug(sentence)
	case "logrus":
		logrus.Debug(sentence)
	default:
	}
}

func (self *Logging) Error(sentence string) {
	switch self.Logger {
	case "go-logging":
		gologging.Error(sentence)
	case "logrus":
		logrus.Error(sentence)
	default:
	}
}

func (self *Logging) Fatal(sentence string) {
	switch self.Logger {
	case "go-logging":
		gologging.Fatal(sentence)
	case "logrus":
		logrus.Fatal(sentence)
	default:
	}
}
