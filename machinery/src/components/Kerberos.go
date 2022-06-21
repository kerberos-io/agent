package components

import (
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/joy4/av/pubsub"
)

func Bootstrap(configuration *models.Configuration, communication *models.Communication) {
	log.Log.Debug("Bootstrap: started")

	// Initiate the packet counter, this is being used to detect
	// if a camera is going blocky, or got disconnected.
	var packageCounter atomic.Value
	packageCounter.Store(int64(0))
	communication.PackageCounter = &packageCounter
	communication.HandleStream = make(chan string, 1)
	communication.HandleMotion = make(chan string, 1)

	// Before starting the agent, we have a control goroutine, that might
	// do several checks to see if the agent is still operational.
	go ControlAgent(communication)

	// Run the agent and fire up all the other
	// goroutines which do image capture, motion detection, onvif, etc.

	for {
		status := RunAgent(configuration, communication)
		if status == "stop" {
			break
		}
	}
	log.Log.Debug("Bootstrap: finished")
}

func RunAgent(configuration *models.Configuration, communication *models.Communication) string {
	log.Log.Debug("RunAgent: started")

	config := configuration.Config

	// Establishing the camera connection
	log.Log.Info("RunAgent: opening RTSP stream")
	rtspUrl := config.Capture.IPCamera.RTSP
	infile, streams, err := OpenRTSP(rtspUrl)

	//var decoder *ffmpeg.VideoDecoder
	var queue *pubsub.Queue

	if err == nil {

		//wg := sync.WaitGroup{}
		queue = pubsub.NewQueue()
		queue.SetMaxGopCount(5) // GOP time frame is set to 5.
		queue.WriteHeader(streams)

		// Start handling the stream
		go HandleStream(infile, queue, communication) //, &wg)

		// Start processing of motion
		motionCursor := queue.Oldest()
		var decoderMutex sync.Mutex
		decoder := GetVideoDecoder(streams)
		go ProcessMotion(motionCursor, configuration, communication, decoder, &decoderMutex)

	} else {
		log.Log.Error("Something went wrong while opening RTSP: " + err.Error())
	}

	// This will go into a blocking state, once this channel is triggered
	// the agent will cleanup and restart.
	status := <-communication.HandleBootstrap

	// Here we are cleaning up everything!
	communication.HandleStream <- "stop"
	infile.Close()
	queue.Close()
	log.Log.Debug("RunAgent: finished")

	return status
}

func ControlAgent(communication *models.Communication) {
	log.Log.Debug("ControlAgent: started")
	packageCounter := communication.PackageCounter
	go func() {
		// A channel to check the camera activity
		var previousPacket int64 = 0
		var occurence = 0
		for {
			packetsR := packageCounter.Load().(int64)
			if packetsR == previousPacket {
				occurence = occurence + 1
			} else {
				occurence = 0
			}

			log.Log.Info("Number of packets read " + strconv.FormatInt(packetsR, 10))
			// After 15 seconds without activity this is thrown..
			if occurence == 3 {
				log.Log.Info("Main: Restarting machinery.")
				communication.HandleBootstrap <- "restart"
				time.Sleep(2 * time.Second)
				occurence = 0
			}
			previousPacket = packageCounter.Load().(int64)

			time.Sleep(5 * time.Second)
		}
	}()
	log.Log.Debug("ControlAgent: finished")
}
