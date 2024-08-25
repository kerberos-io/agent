package main

import (
	"context"
	"flag"
	"os"
	"time"

	"github.com/kerberos-io/agent/machinery/src/capture"
	"github.com/kerberos-io/agent/machinery/src/components"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/onvif"

	configService "github.com/kerberos-io/agent/machinery/src/config"
	"github.com/kerberos-io/agent/machinery/src/routers"
	"github.com/kerberos-io/agent/machinery/src/utils"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"gopkg.in/DataDog/dd-trace-go.v1/profiler"
)

var VERSION = utils.VERSION

func main() {
	// You might be interested in debugging the agent.
	if os.Getenv("DATADOG_AGENT_ENABLED") == "true" {
		if os.Getenv("DATADOG_AGENT_K8S_ENABLED") == "true" {
			tracer.Start()
			defer tracer.Stop()
		} else {
			service := os.Getenv("DATADOG_AGENT_SERVICE")
			environment := os.Getenv("DATADOG_AGENT_ENVIRONMENT")
			log.Log.Info("Starting Datadog Agent with service: " + service + " and environment: " + environment)
			rules := []tracer.SamplingRule{tracer.RateRule(1)}
			tracer.Start(
				tracer.WithSamplingRules(rules),
				tracer.WithService(service),
				tracer.WithEnv(environment),
			)
			defer tracer.Stop()
			err := profiler.Start(
				profiler.WithService(service),
				profiler.WithEnv(environment),
				profiler.WithProfileTypes(
					profiler.CPUProfile,
					profiler.HeapProfile,
				),
			)
			if err != nil {
				log.Log.Fatal(err.Error())
			}
			defer profiler.Stop()
		}
	}

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
		timeout, err := time.ParseDuration(timeout + "ms")
		if err != nil {
			log.Log.Fatal("main.Main(): could not parse timeout: " + err.Error())
			return
		}
		onvif.Discover(timeout)

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
			// Print Kerberos.io ASCII art
			utils.PrintASCIIArt()

			// Print the environment variables which include "AGENT_" as prefix.
			utils.PrintEnvironmentVariables()

			// Read the config on start, and pass it to the other
			// function and features. Please note that this might be changed
			// when saving or updating the configuration through the REST api or MQTT handler.
			var configuration models.Configuration
			configuration.Name = name
			configuration.Port = port

			// Open this configuration either from Kerberos Agent or Kerberos Factory.
			configService.OpenConfig(configDirectory, &configuration)

			// We will override the configuration with the environment variables
			configService.OverrideWithEnvironmentVariables(&configuration)

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

			go components.Bootstrap(configDirectory, &configuration, &communication, &capture)

			// Start the REST API.
			routers.StartWebserver(configDirectory, &configuration, &communication, &capture)
		}
	default:
		log.Log.Error("main.Main(): Sorry I don't understand :(")
	}
}
