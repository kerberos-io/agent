package log

import (
	"os"

	"github.com/op/go-logging"
	"github.com/sirupsen/logrus"
	"gopkg.in/natefinch/lumberjack.v2"
)

// The logging library being used everywhere.
var Log = Logging{
	Logger: "logrus",
	Level:  "debug",
}

// -----------------
// This a gologging
// -> github.com/op/go-logging

var gologging = logging.MustGetLogger("gologger")

func ConfigureGoLogging() {
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
		Filename: "./data/log/machinery.txt",
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

func ConfigureLogrus() {
	// Log as JSON instead of the default ASCII formatter.
	logrus.SetFormatter(&logrus.JSONFormatter{})

	// Output to stdout instead of the default stderr
	// Can be any io.Writer, see below for File example
	logrus.SetOutput(os.Stdout)

	// Only log the warning severity or above.
	logrus.SetLevel(logrus.InfoLevel)
}

func NewLogger(logger string, level string) *Logging {
	loggy := Logging{
		Logger: logger,
		Level:  level,
	}
	loggy.Init()
	return &loggy
}

type Logging struct {
	Logger string
	Level  string
}

func (self *Logging) Init() {
	switch self.Logger {
	case "go-logging":
		ConfigureGoLogging()
	case "logrus":
		ConfigureLogrus()
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
