package main

import (
	"fmt"
	"github.com/kerberos-io/opensource/machinery/src/routers"
	"log"
	"os"
)

func main() {

	const VERSION = 3.0
	action := os.Args[1]

	switch action {
	case "version":
		log.Printf("%s: %0.1f\n", "You are currrently running Kerberos Open Source", VERSION)
	case "pending-upload":
		name := os.Args[2]
		fmt.Println(name)
		//cloud.PendingUpload(log, name)
	case "discover":
		timeout := os.Args[2]
		fmt.Println(timeout)
		//duration, _ := strconv.Atoi(timeout)
		//capture.Discover(log, time.Duration(duration))
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
	return
}
