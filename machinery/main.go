package main

import (
	"fmt"
	"log"
	"os"

	"github.com/kerberos-io/agent/machinery/src/routers"
)

func main() {

	const VERSION = 3.0
	action := os.Args[1]

	switch action {
	case "version":
		log.Printf("%s: %0.1f\n", "You are currrently running Kerberos Agent", VERSION)
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
			routers.StartMqttListener(name)
			routers.StartWebserver(name, port)
		}
	default:
		fmt.Println("Sorry I don't understand :(")
	}
}
