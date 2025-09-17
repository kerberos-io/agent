package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/kerberos-io/agent/machinery/src/capture"
	"github.com/kerberos-io/agent/machinery/src/components"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
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

	flag.StringVar(&action, "action", "version", "Tell us what you want do 'run' or 'version'")
	flag.StringVar(&configDirectory, "config", ".", "Where is the configuration stored")
	flag.StringVar(&name, "name", "agent", "Provide a name for the agent")
	flag.StringVar(&port, "port", "80", "On which port should the agent run")
	flag.StringVar(&timeout, "timeout", "2000", "Number of milliseconds to wait for the ONVIF discovery to complete")
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
	log.Log.Init(logLevel, logOutput, configDirectory, timezone)

	switch action {

	case "version":
		log.Log.Info("main.Main(): You are currrently running Kerberos Agent " + VERSION)

	case "discover":
		// Convert duration to int
		/*timeout, err := time.ParseDuration(timeout + "ms")
		if err != nil {
			log.Log.Fatal("main.Main(): could not parse timeout: " + err.Error())
			return
		}
		onvif.Discover(timeout)*/

	case "decrypt":
		log.Log.Info("main.Main(): Decrypting: " + flag.Arg(0) + " with key: " + flag.Arg(1))
		symmetricKey := []byte(flag.Arg(1))

		if symmetricKey == nil || len(symmetricKey) == 0 {
			log.Log.Fatal("main.Main(): symmetric key should not be empty")
			return
		}
		if len(symmetricKey) != 32 {
			log.Log.Fatal("main.Main(): symmetric key should be 32 bytes")
			return
		}

		utils.Decrypt(flag.Arg(0), symmetricKey)

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
				log.Log.Info("main.Main(): No OpenTelemetry endpoint provided, skipping tracing")
			} else {
				log.Log.Info("main.Main(): Starting OpenTelemetry tracing with endpoint: " + otelEndpoint)
				agentKey := configuration.Config.Key
				traceProvider, err := startTracing(agentKey, otelEndpoint)
				if err != nil {
					log.Log.Error("traceprovider: " + err.Error())
				}
				defer func() {
					if err := traceProvider.Shutdown(context.Background()); err != nil {
						log.Log.Error("traceprovider: " + err.Error())
					}
				}()
			}

			// Printing final configuration
			utils.PrintConfiguration(&configuration)

			// Check the folder permissions, it might be that we do not have permissions to write
			// recordings, update the configuration or save snapshots.
			utils.CheckDataDirectoryPermissions(configDirectory)

			// Set timezone
			timezone, _ := time.LoadLocation(configuration.Config.Timezone)
			log.Log.Init(logLevel, logOutput, configDirectory, timezone)

			// Check if we have a device Key or not, if not
			// we will generate one.
			if configuration.Config.Key == "" {
				key := utils.RandStringBytesMaskImpr(30)
				configuration.Config.Key = key
				err := configService.StoreConfig(configDirectory, configuration.Config)
				if err == nil {
					log.Log.Info("main.Main(): updated unique key for agent to: " + key)
				} else {
					log.Log.Info("main.Main(): something went wrong while trying to store key: " + key)
				}
			}

			// Create a cancelable context, which will be used to cancel and restart.
			// This is used to restart the agent when the configuration is updated.
			ctx, cancel := context.WithCancel(context.Background())

			// We create a capture object, this will contain all the streaming clients.
			// And allow us to extract media from within difference places in the agent.
			capture := capture.Capture{
				RTSPClient:    nil,
				RTSPSubClient: nil,
			}

			// Bootstrapping the agent
			communication := models.Communication{
				Context:         &ctx,
				CancelContext:   &cancel,
				HandleBootstrap: make(chan string, 1),
			}

			go components.Bootstrap(ctx, configDirectory, &configuration, &communication, &capture)

			// Start the REST API.
			routers.StartWebserver(configDirectory, &configuration, &communication, &capture)
		}
	default:
		log.Log.Error("main.Main(): Sorry I don't understand :(")
	}
}
