package main

import (
	"fmt"
	"os"
	"time"

	"github.com/kerberos-io/agent/machinery/src/components"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/routers"
	"github.com/kerberos-io/agent/machinery/src/utils"
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

	case "run":
		{
			name := os.Args[2]
			port := os.Args[3]

			// Check the folder permissions, it might be that we do not have permissions to write
			// recordings, update the configuration or save snapshots.
			err := utils.CheckDataDirectoryPermissions()
			if err != nil {
				log.Log.Fatal(err.Error())
			}

			// Read the config on start, and pass it to the other
			// function and features. Please note that this might be changed
			// when saving or updating the configuration through the REST api or MQTT handler.
			var configuration models.Configuration
			configuration.Name = name
			configuration.Port = port

			// Open this configuration either from Kerberos Agent or Kerberos Factory.
			components.OpenConfig(&configuration)

			// We will override the configuration with the environment variables
			components.OverrideWithEnvironmentVariables(&configuration)

			// Set timezone
			timezone, _ := time.LoadLocation(configuration.Config.Timezone)
			log.Log.Init(timezone)

			// Check if we have a device Key or not, if not
			// we will generate one.
			if configuration.Config.Key == "" {
				key := utils.RandStringBytesMaskImpr(30)
				configuration.Config.Key = key
				err := components.StoreConfig(configuration.Config)
				if err == nil {
					log.Log.Info("Main: updated unique key for agent to: " + key)
				} else {
					log.Log.Info("Main: something went wrong while trying to store key: " + key)
				}
			}

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
