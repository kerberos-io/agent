package components

import (
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kerberos-io/agent/machinery/src/capture"
	"github.com/kerberos-io/agent/machinery/src/cloud"
	"github.com/kerberos-io/agent/machinery/src/computervision"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/onvif"
	routers "github.com/kerberos-io/agent/machinery/src/routers/mqtt"
	"github.com/kerberos-io/joy4/av/pubsub"
	"github.com/tevino/abool"
)

func Bootstrap(configuration *models.Configuration, communication *models.Communication) {
	log.Log.Debug("Bootstrap: started")

	// Initiate the packet counter, this is being used to detect
	// if a camera is going blocky, or got disconnected.
	var packageCounter atomic.Value
	packageCounter.Store(int64(0))
	communication.PackageCounter = &packageCounter
	communication.HandleStream = make(chan string, 1)
	communication.HandleUpload = make(chan string, 1)
	communication.HandleHeartBeat = make(chan string, 1)
	communication.HandleLiveSD = make(chan int64, 1)
	communication.HandleLiveHDKeepalive = make(chan string, 1)
	communication.HandleLiveHDPeers = make(chan string, 1)
	communication.IsConfiguring = abool.New()

	// Before starting the agent, we have a control goroutine, that might
	// do several checks to see if the agent is still operational.
	go ControlAgent(communication)

	// Run the agent and fire up all the other
	// goroutines which do image capture, motion detection, onvif, etc.

	for {
		// This will blocking until receiving a signal to be restarted, reconfigured, stopped, etc.
		status := RunAgent(configuration, communication)
		if status == "stop" {
			break
		}
		// We will re open the configuration, might have changed :O!
		OpenConfig(configuration)
	}
	log.Log.Debug("Bootstrap: finished")
}

func RunAgent(configuration *models.Configuration, communication *models.Communication) string {
	log.Log.Debug("RunAgent: started")

	config := configuration.Config

	// Currently only support H264 encoded cameras, this will change.
	// Establishing the camera connection
	log.Log.Info("RunAgent: opening RTSP stream")
	rtspUrl := config.Capture.IPCamera.RTSP
	infile, streams, err := capture.OpenRTSP(rtspUrl)

	//var decoder *ffmpeg.VideoDecoder
	var queue *pubsub.Queue
	status := "not started"

	if err == nil {

		// At some routines we will need to decode the image.
		// Make sure its properly locked as we only have a single decoder.
		var decoderMutex sync.Mutex
		decoder := capture.GetVideoDecoder(streams)

		// Create a packet queue, which is filled by the HandleStream routing
		// and consumed by all other routines: motion, livestream, etc.
		queue = pubsub.NewQueue()
		queue.SetMaxGopCount(5) // GOP time frame is set to 5.
		queue.WriteHeader(streams)

		// Configure a MQTT client which helps for a bi-directional communication
		communication.HandleONVIF = make(chan models.OnvifAction, 1)
		mqttClient := routers.ConfigureMQTT(configuration, communication)

		// Handle heartbeats
		go cloud.HandleHeartBeat(configuration, communication)

		// Handle the camera stream
		go capture.HandleStream(infile, queue, communication) //, &wg)

		// Handle processing of motion
		motionCursor := queue.Oldest()
		communication.HandleMotion = make(chan int64, 1)
		go computervision.ProcessMotion(motionCursor, configuration, communication, mqttClient, decoder, &decoderMutex)

		// Handle livestream SD (low resolution over MQTT)
		livestreamCursor := queue.Oldest()
		go cloud.HandleLiveStreamSD(livestreamCursor, configuration, communication, mqttClient, decoder, &decoderMutex)

		// Handle livestream HD (high resolution over WEBRTC)
		livestreamHDCursor := queue.Oldest()
		communication.HandleLiveHDHandshake = make(chan models.SDPPayload, 1)
		go cloud.HandleLiveStreamHD(livestreamHDCursor, configuration, communication, mqttClient, streams, decoder, &decoderMutex)

		// Handle recording, will write an mp4 to disk.
		recordingCursor := queue.Oldest()
		go capture.HandleRecordStream(recordingCursor, configuration, communication, streams)

		// Handle Upload to cloud provider (Kerberos Hub, Kerberos Vault and others)
		go cloud.HandleUpload(configuration, communication)

		// Handle ONVIF actions
		go onvif.HandleONVIFActions(configuration, communication)

		// !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
		// This will go into a blocking state, once this channel is triggered
		// the agent will cleanup and restart.
		status = <-communication.HandleBootstrap

		// Here we are cleaning up everything!
		communication.HandleStream <- "stop"
		communication.HandleHeartBeat <- "stop"
		communication.HandleUpload <- "stop"
		infile.Close()
		queue.Close()
		close(communication.HandleONVIF)
		close(communication.HandleLiveHDHandshake)
		close(communication.HandleMotion)
		routers.DisconnectMQTT(mqttClient)
		decoder.Close()

		// Waiting for some seconds to make sure everything is properly closed.
		log.Log.Info("RunAgent: waiting 1 second to make sure everything is properly closed.")
		time.Sleep(time.Second * 1)
	} else {
		log.Log.Error("Something went wrong while opening RTSP: " + err.Error())
		time.Sleep(time.Second * 3)
	}

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
				// If we are already reconfiguring,
				// we dont need to check if the stream is blocking.
				if !communication.IsConfiguring.IsSet() {
					occurence = occurence + 1
				}
			} else {
				occurence = 0
			}

			log.Log.Info("ControlAgent: Number of packets read " + strconv.FormatInt(packetsR, 10))

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
