package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/kerberos-io/agent/machinery/src/capture"
	"github.com/kerberos-io/agent/machinery/src/components"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/onvif"
	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"

	configService "github.com/kerberos-io/agent/machinery/src/config"
	"github.com/kerberos-io/agent/machinery/src/routers"
	"github.com/kerberos-io/agent/machinery/src/utils"
)

var VERSION = utils.VERSION

func resolveServerPort(flagValue, environmentValue string) (string, error) {
	value := strings.TrimSpace(environmentValue)
	if value == "" {
		value = strings.TrimSpace(flagValue)
	}
	if value == "" {
		value = "80"
	}

	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return "", fmt.Errorf("port must be an integer between 1 and 65535, got %q", value)
	}
	return strconv.Itoa(port), nil
}

func startTracing(agentKey string, otelEndpoint string) (*trace.TracerProvider, error) {
	serviceName := "agent-" + agentKey
	headers := map[string]string{
		"content-type": "application/json",
	}

	exporter, err := otlptrace.New(
		context.Background(),
		otlptracehttp.NewClient(
			otlptracehttp.WithEndpoint(otelEndpoint),
			otlptracehttp.WithHeaders(headers),
			otlptracehttp.WithInsecure(),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating new exporter: %w", err)
	}

	tracerprovider := trace.NewTracerProvider(
		trace.WithBatcher(
			exporter,
			trace.WithMaxExportBatchSize(trace.DefaultMaxExportBatchSize),
			trace.WithBatchTimeout(trace.DefaultScheduleDelay*time.Millisecond),
			trace.WithMaxExportBatchSize(trace.DefaultMaxExportBatchSize),
		),
		trace.WithResource(
			resource.NewWithAttributes(
				semconv.SchemaURL,
				semconv.ServiceNameKey.String(serviceName),
				attribute.String("environment", "develop"),
			),
		),
	)

	otel.SetTracerProvider(tracerprovider)

	return tracerprovider, nil
}

func main() {

	// Start the show ;)
	// We'll parse the flags (named variables), and start the agent.

	var action string
	var configDirectory string
	var name string
	var port string
	var timeout string
	var subnet string

	flag.StringVar(&action, "action", "version", "Tell us what you want do 'run' or 'version'")
	flag.StringVar(&configDirectory, "config", ".", "Where is the configuration stored")
	flag.StringVar(&name, "name", "agent", "Provide a name for the agent")
	flag.StringVar(&port, "port", "80", "On which port should the agent run")
	flag.StringVar(&timeout, "timeout", "2000", "Number of milliseconds to wait for the ONVIF discovery to complete")
	flag.StringVar(&subnet, "subnet", "", "Optional subnet(s) to scan for discovery, e.g. '192.168.1.0/24' (comma-separated). Defaults to the local interfaces.")
	flag.Parse()

	// Specify the level of loggin: "info", "warning", "debug", "error" or "fatal."
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	// Specify the output formatter of the log: "text" or "json".
	logOutput := os.Getenv("LOG_OUTPUT")
	if logOutput == "" {
		logOutput = "text"
	}
	// Specify the timezone of the log: "UTC" or "Local".
	timezone, _ := time.LoadLocation("CET")
	configureLogging(logLevel, logOutput, timezone)
	if action == "run" {
		resolvedPort, err := resolveServerPort(port, os.Getenv("AGENT_PORT"))
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"component": "http",
				"event":     "server_port_invalid",
			}).Fatal("Invalid HTTP server port")
			return
		}
		port = resolvedPort
	}
	log.WithFields(log.Fields{
		"action":           action,
		"component":        "agent",
		"config_directory": configDirectory,
		"event":            "command_parsed",
		"port":             port,
		"version":          VERSION,
	}).Debug("Agent command parsed")

	switch action {

	case "version":
		{
			log.WithFields(log.Fields{
				"component": "agent",
				"event":     "version",
				"version":   VERSION,
			}).Info("Kerberos Agent version")
		}
	case "discover":
		{
			// Convert duration to int
			timeout, err := time.ParseDuration(timeout + "ms")
			if err != nil {
				log.WithError(err).WithField("component", "onvif").
					Fatal("invalid ONVIF discovery timeout")
				return
			}
			var subnets []string
			for _, part := range strings.Split(subnet, ",") {
				if trimmed := strings.TrimSpace(part); trimmed != "" {
					subnets = append(subnets, trimmed)
				}
			}
			onvif.Discover(timeout, subnets...)
		}
	case "decrypt":
		{
			log.WithFields(log.Fields{
				"component": "encryption",
				"event":     "decrypt_started",
				"path":      flag.Arg(0),
			}).Info("Decrypting recording")
			symmetricKey := []byte(flag.Arg(1))

			if len(symmetricKey) == 0 {
				log.Fatal("main.Main(): symmetric key should not be empty")
				return
			}
			if len(symmetricKey) != 32 {
				log.Fatal("main.Main(): symmetric key should be 32 bytes")
				return
			}

			utils.Decrypt(flag.Arg(0), symmetricKey)
		}

	case "run":
		{
			// Print Agent ASCII art
			utils.PrintASCIIArt()

			// Print the environment variables which include "AGENT_" as prefix.
			utils.PrintEnvironmentVariables()

			// Read the config on start, and pass it to the other
			// function and features. Please note that this might be changed
			// when saving or updating the configuration through the REST api or MQTT handler.
			var configuration models.Configuration
			configuration.Name = name
			configuration.Port = port

			// Open this configuration either from Agent or Factory.
			configService.OpenConfig(configDirectory, &configuration)

			// We will override the configuration with the environment variables
			configService.OverrideWithEnvironmentVariables(&configuration)

			// Start OpenTelemetry tracing
			if otelEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); otelEndpoint == "" {
				log.WithFields(log.Fields{
					"component": "tracing",
					"event":     "tracing_disabled",
				}).Debug("OpenTelemetry tracing disabled")
			} else {
				log.WithFields(log.Fields{
					"component": "tracing",
					"event":     "tracing_starting",
				}).Info("Starting OpenTelemetry tracing")
				agentKey := configuration.Config.Key
				traceProvider, err := startTracing(agentKey, otelEndpoint)
				if err != nil {
					log.WithError(err).WithField("component", "tracing").
						Error("Failed to start OpenTelemetry tracing")
				} else {
					defer func() {
						if err := traceProvider.Shutdown(context.Background()); err != nil {
							log.WithError(err).WithField("component", "tracing").
								Error("Failed to shut down OpenTelemetry tracing")
						}
					}()
				}
			}

			// Printing final configuration
			utils.PrintConfiguration(&configuration)

			// Check the folder permissions, it might be that we do not have permissions to write
			// recordings, update the configuration or save snapshots.
			utils.CheckDataDirectoryPermissions(configDirectory)

			// Set timezone
			timezone, err := time.LoadLocation(configuration.Config.Timezone)
			if err != nil {
				log.WithError(err).WithField("timezone", configuration.Config.Timezone).
					Warn("invalid Agent timezone; using the host timezone for logs")
				timezone = time.Local
			}
			configureLogging(logLevel, logOutput, timezone)

			// Check if we have a device Key or not, if not
			// we will generate one.
			if configuration.Config.Key == "" {
				key := utils.RandStringBytesMaskImpr(30)
				configuration.Config.Key = key
				err := configService.StoreConfig(configDirectory, configuration.Config)
				if err == nil {
					log.WithFields(log.Fields{
						"component": "configuration",
						"event":     "agent_key_generated",
					}).Info("Generated and stored a unique Agent key")
				} else {
					log.WithError(err).WithFields(log.Fields{
						"component": "configuration",
						"event":     "agent_key_store_failed",
					}).Error("Failed to store the generated Agent key")
				}
			}

			// Create a cancelable context, which will be used to cancel and restart.
			// This is used to restart the agent when the configuration is updated.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// We create a capture object, this will contain all the streaming clients.
			// And allow us to extract media from within difference places in the agent.
			capture := capture.Capture{}

			// Bootstrapping the agent
			communication := models.Communication{
				HandleBootstrap: make(chan string, 1),
			}

			log.WithFields(log.Fields{
				"component": "agent",
				"event":     "runtime_starting",
				"port":      configuration.Port,
			}).Info("Starting Agent runtime")
			go components.Bootstrap(ctx, configDirectory, &configuration, &communication, &capture)

			// Start the REST API.
			routers.StartWebserver(configDirectory, &configuration, &communication, &capture)
		}
	default:
		{
			log.Error("main.Main(): Sorry I don't understand :(")
		}
	}
}
