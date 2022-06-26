package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/kerberos-io/agent/machinery/src/components"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/routers"
	"gocv.io/x/gocv"
)

func main() {

	const VERSION = "3.0"
	action := os.Args[1]

	log.Log.Init()

	switch action {
	case "version":
		log.Log.Info("You are currrently running Kerberos Agent " + VERSION)

	case "pending-upload":
		name := os.Args[2]
		fmt.Println(name)

	case "discover":
		timeout := os.Args[2]
		fmt.Println(timeout)

	case "webcam-test":

		deviceID := os.Args[2]
		webcam, err := gocv.OpenVideoCapture(deviceID)
		if err != nil {
			fmt.Printf("Error opening video capture device: %v\n", deviceID)
			return
		}
		defer webcam.Close()
		buf := gocv.NewMat()
		defer buf.Close()

		ok := webcam.Read(&buf)

		if ok {

			now := time.Now().Unix()
			saveFile := "./data/capture-test/" + strconv.FormatInt(now, 10) + ".mp4"
			fps := 20.0 // webcam.Get(gocv.VideoCaptureFPS)
			writer, err := gocv.VideoWriterFile(saveFile, "X264", fps, buf.Cols(), buf.Rows(), true)
			if err != nil {
				fmt.Println(err)
				fmt.Printf("error opening video writer device: %v\n", saveFile)
				return
			}
			defer writer.Close()

			//window := gocv.NewWindow("Hello")
			fmt.Printf("Start reading device: %v\n", deviceID)
			for i := 0; i < 100; i++ {
				if ok := webcam.Read(&buf); !ok {
					fmt.Printf("Device closed: %v\n", deviceID)
					return
				}
				if buf.Empty() {
					continue
				}

				writer.Write(buf)
				//window.IMShow(buf)
				//window.WaitKey(1)

				fmt.Printf("Read frame %d\n", i+1)
			}

		}
		fmt.Println("Done.")

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
