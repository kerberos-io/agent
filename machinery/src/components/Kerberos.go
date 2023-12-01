package components

import (
	"context"
	"strconv"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/kerberos-io/agent/machinery/src/capture"
	"github.com/kerberos-io/agent/machinery/src/cloud"
	"github.com/kerberos-io/agent/machinery/src/computervision"
	configService "github.com/kerberos-io/agent/machinery/src/config"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/onvif"
	"github.com/kerberos-io/agent/machinery/src/packets"
	routers "github.com/kerberos-io/agent/machinery/src/routers/mqtt"
	"github.com/tevino/abool"
)

func Bootstrap(configDirectory string, configuration *models.Configuration, communication *models.Communication, captureDevice *capture.Capture) {
	log.Log.Debug("Bootstrap: started")

	// We will keep track of the Kerberos Agent up time
	// This is send to Kerberos Hub in a heartbeat.
	uptimeStart := time.Now()

	// Initiate the packet counter, this is being used to detect
	// if a camera is going blocky, or got disconnected.
	var packageCounter atomic.Value
	packageCounter.Store(int64(0))
	communication.PackageCounter = &packageCounter

	// This is used when the last packet was received (timestamp),
	// this metric is used to determine if the camera is still online/connected.
	var lastPacketTimer atomic.Value
	packageCounter.Store(int64(0))
	communication.LastPacketTimer = &lastPacketTimer

	// This is used to understand if we have a working Kerberos Hub connection
	// cloudTimestamp will be updated when successfully sending heartbeats.
	var cloudTimestamp atomic.Value
	cloudTimestamp.Store(int64(0))
	communication.CloudTimestamp = &cloudTimestamp

	communication.HandleStream = make(chan string, 1)
	communication.HandleSubStream = make(chan string, 1)
	communication.HandleUpload = make(chan string, 1)
	communication.HandleHeartBeat = make(chan string, 1)
	communication.HandleLiveSD = make(chan int64, 1)
	communication.HandleLiveHDKeepalive = make(chan string, 1)
	communication.HandleLiveHDPeers = make(chan string, 1)
	communication.HandleONVIF = make(chan models.OnvifAction, 1)
	communication.IsConfiguring = abool.New()

	cameraSettings := &models.Camera{}

	// Before starting the agent, we have a control goroutine, that might
	// do several checks to see if the agent is still operational.
	go ControlAgent(communication)

	// Handle heartbeats
	go cloud.HandleHeartBeat(configuration, communication, uptimeStart)

	// We'll create a MQTT handler, which will be used to communicate with Kerberos Hub.
	// Configure a MQTT client which helps for a bi-directional communication
	mqttClient := routers.ConfigureMQTT(configDirectory, configuration, communication)

	// Run the agent and fire up all the other
	// goroutines which do image capture, motion detection, onvif, etc.
	for {

		// This will blocking until receiving a signal to be restarted, reconfigured, stopped, etc.
		status := RunAgent(configDirectory, configuration, communication, mqttClient, uptimeStart, cameraSettings, captureDevice)

		if status == "stop" {
			break
		}

		if status == "not started" {
			// We will re open the configuration, might have changed :O!
			configService.OpenConfig(configDirectory, configuration)
			// We will override the configuration with the environment variables
			configService.OverrideWithEnvironmentVariables(configuration)
		}

		// Reset the MQTT client, might have provided new information, so we need to reconnect.
		if routers.HasMQTTClientModified(configuration) {
			routers.DisconnectMQTT(mqttClient, &configuration.Config)
			mqttClient = routers.ConfigureMQTT(configDirectory, configuration, communication)
		}

		// We will create a new cancelable context, which will be used to cancel and restart.
		// This is used to restart the agent when the configuration is updated.
		ctx, cancel := context.WithCancel(context.Background())
		communication.Context = &ctx
		communication.CancelContext = &cancel
	}
	log.Log.Debug("Bootstrap: finished")
}

func RunAgent(configDirectory string, configuration *models.Configuration, communication *models.Communication, mqttClient mqtt.Client, uptimeStart time.Time, cameraSettings *models.Camera, captureDevice *capture.Capture) string {

	log.Log.Debug("RunAgent: bootstrapping agent")
	config := configuration.Config

	status := "not started"

	// Currently only support H264 encoded cameras, this will change.
	// Establishing the camera connection without backchannel if no substream
	rtspUrl := config.Capture.IPCamera.RTSP
	withBackChannel := true
	rtspClient := captureDevice.SetMainClient(rtspUrl, withBackChannel)

	err := rtspClient.Connect(context.Background())
	if err != nil {
		log.Log.Error("RunAgent: error connecting to RTSP stream: " + err.Error())
		time.Sleep(time.Second * 3)
		return status
	}

	// Check if has backchannel, then we set it in the communication struct
	communication.HasBackChannel = rtspClient.HasBackChannel

	// Get the video streams from the RTSP server.
	videoStreams, err := rtspClient.GetVideoStreams()
	if err != nil || len(videoStreams) == 0 {
		log.Log.Error("RunAgent: no video stream found, might be the wrong codec (we only support H264 for the moment)")
		time.Sleep(time.Second * 3)
		return status
	}

	// Get the video stream from the RTSP server.
	videoStream := videoStreams[0]

	// Get some information from the video stream.
	//	num := videoStream.Num
	//denum := videoStream.Denum
	width := videoStream.Width
	height := videoStream.Height

	// Set config values as well
	configuration.Config.Capture.IPCamera.Width = width
	configuration.Config.Capture.IPCamera.Height = height

	var queue *packets.Queue
	var subQueue *packets.Queue

	log.Log.Info("RunAgent: opened RTSP stream: " + rtspUrl)

	// Create a packet queue, which is filled by the HandleStream routing
	// and consumed by all other routines: motion, livestream, etc.
	if config.Capture.PreRecording <= 0 {
		config.Capture.PreRecording = 1
		log.Log.Warning("RunAgent: Prerecording value not found in config or invalid value! Found: " + strconv.FormatInt(config.Capture.PreRecording, 10))
	}

	// We might have a secondary rtsp url, so we might need to use that for livestreaming let us check first!
	subStreamEnabled := false
	subRtspUrl := config.Capture.IPCamera.SubRTSP
	var videoSubStreams []packets.Stream

	if subRtspUrl != "" && subRtspUrl != rtspUrl {
		// For the sub stream we will not enable backchannel.
		subStreamEnabled = true
		withBackChannel := false
		rtspSubClient := captureDevice.SetSubClient(subRtspUrl, withBackChannel)
		captureDevice.RTSPSubClient = rtspSubClient

		err := rtspSubClient.Connect(context.Background())
		if err != nil {
			log.Log.Error("RunAgent: error connecting to RTSP sub stream: " + err.Error())
			time.Sleep(time.Second * 3)
			return status
		}

		// Get the video streams from the RTSP server.
		videoSubStreams, err = rtspSubClient.GetVideoStreams()
		if err != nil || len(videoSubStreams) == 0 {
			log.Log.Error("RunAgent: no video sub stream found, might be the wrong codec (we only support H264 for the moment)")
			time.Sleep(time.Second * 3)
			return status
		}

		// Get the video stream from the RTSP server.
		videoSubStream := videoSubStreams[0]

		width := videoSubStream.Width
		height := videoSubStream.Height

		// Set config values as well
		configuration.Config.Capture.IPCamera.Width = width
		configuration.Config.Capture.IPCamera.Height = height
	}

	if cameraSettings.RTSP != rtspUrl ||
		cameraSettings.SubRTSP != subRtspUrl ||
		cameraSettings.Width != width ||
		cameraSettings.Height != height {
		//cameraSettings.Num != num ||
		//cameraSettings.Denum != denum ||
		//cameraSettings.Codec != videoStream.(av.VideoCodecData).Type() {

		// TODO: this condition is used to reset the decoder when the camera settings change.
		// The main idea is that you only set the decoder once, and then reuse it on each restart (no new memory allocation).
		// However the stream settings of the camera might have been changed, and so the decoder might need to be reloaded.
		// ....

		if cameraSettings.RTSP != "" && cameraSettings.SubRTSP != "" && cameraSettings.Initialized {
			//decoder.Close()
			//if subStreamEnabled {
			//	subDecoder.Close()
			//}
		}

		// At some routines we will need to decode the image.
		// Make sure its properly locked as we only have a single decoder.
		log.Log.Info("RunAgent: camera settings changed, reloading decoder")
		//capture.GetVideoDecoder(decoder, streams)
		//if subStreamEnabled {
		//	capture.GetVideoDecoder(subDecoder, subStreams)
		//}

		cameraSettings.RTSP = rtspUrl
		cameraSettings.SubRTSP = subRtspUrl
		cameraSettings.Width = width
		cameraSettings.Height = height
		//cameraSettings.Framerate = float64(num) / float64(denum)
		//cameraSettings.Num = num
		//cameraSettings.Denum = denum
		//cameraSettings.Codec = videoStream.(av.VideoCodecData).Type()
		cameraSettings.Initialized = true
	} else {
		log.Log.Info("RunAgent: camera settings did not change, keeping decoder")
	}

	// We are creating a queue to store the RTSP frames in, these frames will be
	// processed by the different consumers: motion detection, recording, etc.
	queue = packets.NewQueue()
	communication.Queue = queue

	// Set the maximum GOP count, this is used to determine the pre-recording time.
	log.Log.Info("RunAgent: SetMaxGopCount was set with: " + strconv.Itoa(int(config.Capture.PreRecording)+1))
	queue.SetMaxGopCount(int(config.Capture.PreRecording) + 1) // GOP time frame is set to prerecording (we'll add 2 gops to leave some room).
	queue.WriteHeader(videoStreams)
	go rtspClient.Start(context.Background(), queue, communication)

	rtspSubClient := captureDevice.RTSPSubClient
	if subStreamEnabled && rtspSubClient != nil {
		subQueue = packets.NewQueue()
		communication.SubQueue = subQueue
		subQueue.SetMaxGopCount(1) // GOP time frame is set to prerecording (we'll add 2 gops to leave some room).
		subQueue.WriteHeader(videoSubStreams)
		go rtspSubClient.Start(context.Background(), subQueue, communication)
	}

	// Handle livestream SD (low resolution over MQTT)
	if subStreamEnabled {
		livestreamCursor := subQueue.Latest()
		go cloud.HandleLiveStreamSD(livestreamCursor, configuration, communication, mqttClient, rtspSubClient)
	} else {
		livestreamCursor := queue.Latest()
		go cloud.HandleLiveStreamSD(livestreamCursor, configuration, communication, mqttClient, rtspClient)
	}

	// Handle livestream HD (high resolution over WEBRTC)
	communication.HandleLiveHDHandshake = make(chan models.RequestHDStreamPayload, 1)
	if subStreamEnabled {
		livestreamHDCursor := subQueue.Latest()
		go cloud.HandleLiveStreamHD(livestreamHDCursor, configuration, communication, mqttClient, rtspSubClient)
	} else {
		livestreamHDCursor := queue.Latest()
		go cloud.HandleLiveStreamHD(livestreamHDCursor, configuration, communication, mqttClient, rtspClient)
	}

	// Handle recording, will write an mp4 to disk.
	go capture.HandleRecordStream(queue, configDirectory, configuration, communication, rtspClient)

	// Handle processing of motion
	communication.HandleMotion = make(chan models.MotionDataPartial, 1)
	if subStreamEnabled {
		motionCursor := subQueue.Latest()
		go computervision.ProcessMotion(motionCursor, configuration, communication, mqttClient, rtspSubClient)
	} else {
		motionCursor := queue.Latest()
		go computervision.ProcessMotion(motionCursor, configuration, communication, mqttClient, rtspClient)
	}

	// Handle Upload to cloud provider (Kerberos Hub, Kerberos Vault and others)
	go cloud.HandleUpload(configDirectory, configuration, communication)

	// Handle ONVIF actions
	go onvif.HandleONVIFActions(configuration, communication)

	communication.HandleAudio = make(chan models.AudioDataPartial, 1)
	if rtspClient.HasBackChannel {
		go WriteAudioToBackchannel(communication, rtspClient)
	}

	// If we reach this point, we have a working RTSP connection.
	communication.CameraConnected = true

	// !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
	// This will go into a blocking state, once this channel is triggered
	// the agent will cleanup and restart.

	status = <-communication.HandleBootstrap

	// If we reach this point, we are stopping the stream.
	communication.CameraConnected = false

	// Cancel the main context, this will stop all the other goroutines.
	(*communication.CancelContext)()

	// We will re open the configuration, might have changed :O!
	configService.OpenConfig(configDirectory, configuration)

	// We will override the configuration with the environment variables
	configService.OverrideWithEnvironmentVariables(configuration)

	// Here we are cleaning up everything!
	if configuration.Config.Offline != "true" {
		communication.HandleUpload <- "stop"
	}
	communication.HandleStream <- "stop"
	// We use the steam channel to stop both main and sub stream.
	//if subStreamEnabled {
	//	communication.HandleSubStream <- "stop"
	//}

	time.Sleep(time.Second * 3)

	err = rtspClient.Close()
	if err != nil {
		log.Log.Error("RunAgent: error closing RTSP stream: " + err.Error())
		time.Sleep(time.Second * 3)
		return status
	}

	queue.Close()
	queue = nil
	communication.Queue = nil
	if subStreamEnabled {
		err = rtspSubClient.Close()
		if err != nil {
			log.Log.Error("RunAgent: error closing RTSP sub stream: " + err.Error())
			time.Sleep(time.Second * 3)
			return status
		}
		subQueue.Close()
		subQueue = nil
		communication.SubQueue = nil
	}

	close(communication.HandleMotion)
	communication.HandleMotion = nil
	//close(communication.HandleAudio)
	//communication.HandleAudio = nil

	// Waiting for some seconds to make sure everything is properly closed.
	log.Log.Info("RunAgent: waiting 3 seconds to make sure everything is properly closed.")
	time.Sleep(time.Second * 3)

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

			// If camera is connected, we'll check if we are still receiving packets.
			if communication.CameraConnected {
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
			}

			time.Sleep(5 * time.Second)
		}
	}()
	log.Log.Debug("ControlAgent: finished")
}
