package main

import (
	"fmt"
	"os"
	"time"

	"github.com/kerberos-io/agent/machinery/src/capture"
	"github.com/kerberos-io/agent/machinery/src/components"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/routers"
)

func main() {

	const VERSION = "3.0"
	action := os.Args[1]

	timezone, _ := time.LoadLocation("CET")
	log.Log.Init(timezone)

	switch action {

	case "version":
		log.Log.Info("You are currrently running Kerberos Agent " + VERSION)

	case "pending-upload":
		name := os.Args[2]
		fmt.Println(name)

	case "discover":
		timeout := os.Args[2]
		fmt.Println(timeout)

	case "usbcamera-test":

		deviceID := os.Args[2]
		capture.TestUSBCamera(deviceID)

	case "run":
		{
			name := os.Args[2]
			port := os.Args[3]

			// Read the config on start, and pass it to the other
			// function and features. Please note that this might be changed
			// when saving or updating the configuration through the REST api or MQTT handler.
			var configuration models.Configuration
			configuration.Name = name
			configuration.Port = port

			// Open this configuration either from Kerberos Agent or Kerberos Factory.
			components.OpenConfig(&configuration)

			timezone, _ := time.LoadLocation(configuration.Config.Timezone)
			log.Log.Init(timezone)

			// Bootstrapping the agent
			communication := models.Communication{
				HandleBootstrap: make(chan string, 1),
			}
			go components.Bootstrap(&configuration, &communication)

			// Start the REST API.
			routers.StartWebserver(&configuration, &communication)
		}
	default:
		fmt.Println("Sorry I don't understand :(")
	}
}
