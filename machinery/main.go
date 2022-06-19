package main

import (
	"fmt"
	"os"

	"github.com/kerberos-io/agent/machinery/src/components"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/routers"
)

var log = components.Logging{
	Logger: "logrus",
	Level:  "warning",
}

func main() {

	const VERSION = "3.0"
	action := os.Args[1]

	log.Init()

	switch action {
	case "version":
		log.Info("You are currrently running Kerberos Agent " + VERSION)

	case "pending-upload":
		name := os.Args[2]
		fmt.Println(name)

	case "discover":
		timeout := os.Args[2]
		fmt.Println(timeout)

	case "run":
		{
			name := os.Args[2]
			port := os.Args[3]

			// Read the config on start, and pass it to the other
			// function and features. Please note that this might be changed
			// when saving or updating the configuration through the REST api or MQTT handler.
			var config models.Config
			var customConfig models.Config
			var globalConfig models.Config

			// Open this configuration either from Kerberos Agent or Kerberos Factory.
			components.OpenConfig(name, log, &config, &customConfig, &globalConfig)

			// Bootstrapping the agent
			components.Bootstrap(&config, log)

			// Start a MQTT listener.
			routers.StartMqttListener(name)

			// Start the REST API.
			routers.StartWebserver(name, port, &config, &customConfig, &globalConfig)
		}
	default:
		fmt.Println("Sorry I don't understand :(")
	}
}
