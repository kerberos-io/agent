package components

import (
	"context"
	"strconv"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/kerberos-io/agent/machinery/src/capture"
	"github.com/kerberos-io/agent/machinery/src/cloud"
	configService "github.com/kerberos-io/agent/machinery/src/config"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/onvif"
	"github.com/kerberos-io/agent/machinery/src/packets"
	routers "github.com/kerberos-io/agent/machinery/src/routers/mqtt"
	"github.com/tevino/abool"
)

func Bootstrap(configDirectory string, configuration *models.Configuration, communication *models.Communication) {
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

	// Before starting the agent, we have a control goroutine, that might
	// do several checks to see if the agent is still operational.
	go ControlAgent(communication)

	// Create some global variables
	//decoder := &ffmpeg.VideoDecoder{}
	//subDecoder := &ffmpeg.VideoDecoder{}
	cameraSettings := &models.Camera{}

	// Handle heartbeats
	go cloud.HandleHeartBeat(configuration, communication, uptimeStart)

	// We'll create a MQTT handler, which will be used to communicate with Kerberos Hub.
	// Configure a MQTT client which helps for a bi-directional communication
	mqttClient := routers.ConfigureMQTT(configDirectory, configuration, communication)

	// Run the agent and fire up all the other
	// goroutines which do image capture, motion detection, onvif, etc.
	for {

		// This will blocking until receiving a signal to be restarted, reconfigured, stopped, etc.
		//status := RunAgent(configDirectory, configuration, communication, mqttClient, uptimeStart, cameraSettings, decoder, subDecoder)
		status := RunAgent(configDirectory, configuration, communication, mqttClient, uptimeStart, cameraSettings)

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

func RunAgent(configDirectory string, configuration *models.Configuration, communication *models.Communication, mqttClient mqtt.Client, uptimeStart time.Time, cameraSettings *models.Camera) string {

	log.Log.Debug("RunAgent: bootstrapping agent")
	config := configuration.Config

	status := "not started"

	// Currently only support H264 encoded cameras, this will change.
	// Establishing the camera connection without backchannel if no substream
	rtspUrl := config.Capture.IPCamera.RTSP
	withBackChannel := true
	rtspClient := &capture.Joy4{
		Url:             rtspUrl,
		WithBackChannel: withBackChannel,
	}

	err := rtspClient.Connect(context.Background())
	if err != nil {
		log.Log.Error("RunAgent: error connecting to RTSP stream: " + err.Error())
		time.Sleep(time.Second * 3)
		return status
	}

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
	//var subQueue *packets.Queue

	//var decoderMutex sync.Mutex
	//var subDecoderMutex sync.Mutex
	subStreamEnabled := false

	log.Log.Info("RunAgent: opened RTSP stream: " + rtspUrl)

	// Create a packet queue, which is filled by the HandleStream routing
	// and consumed by all other routines: motion, livestream, etc.
	if config.Capture.PreRecording <= 0 {
		config.Capture.PreRecording = 1
		log.Log.Warning("RunAgent: Prerecording value not found in config or invalid value! Found: " + strconv.FormatInt(config.Capture.PreRecording, 10))
	}

	// TODO add the substream + another check if the resolution changed.

	// We are creating a queue to store the RTSP frames in, these frames will be
	// processed by the different consumers: motion detection, recording, etc.
	queue = packets.NewQueue()
	//communication.Queue = queue

	queue.SetMaxGopCount(int(config.Capture.PreRecording) + 1) // GOP time frame is set to prerecording (we'll add 2 gops to leave some room).
	log.Log.Info("RunAgent: SetMaxGopCount was set with: " + strconv.Itoa(int(config.Capture.PreRecording)+1))
	queue.WriteHeader(videoStreams)

	// Handle the camera stream
	//go capture.HandleStream(infile, queue, communication)
	go rtspClient.Start(context.Background(), queue, communication)

	// Handle livestream SD (low resolution over MQTT)
	if subStreamEnabled {
		//livestreamCursor := subQueue.Latest()
		//go cloud.HandleLiveStreamSD(livestreamCursor, configuration, communication, mqttClient, rtspSubClient)
	} else {
		livestreamCursor := queue.Latest()
		go cloud.HandleLiveStreamSD(livestreamCursor, configuration, communication, mqttClient, rtspClient)
	}

	// Handle livestream HD (high resolution over WEBRTC)
	communication.HandleLiveHDHandshake = make(chan models.RequestHDStreamPayload, 1)
	if subStreamEnabled {
		//livestreamHDCursor := subQueue.Latest()
		//go cloud.HandleLiveStreamHD(livestreamHDCursor, configuration, communication, mqttClient, subStreams, subDecoder, &decoderMutex)
	} else {
		livestreamHDCursor := queue.Latest()
		go cloud.HandleLiveStreamHD(livestreamHDCursor, configuration, communication, mqttClient, rtspClient)
	}

	// Handle recording, will write an mp4 to disk.
	go capture.HandleRecordStream(queue, configDirectory, configuration, communication, rtspClient)

	// Handle Upload to cloud provider (Kerberos Hub, Kerberos Vault and others)
	go cloud.HandleUpload(configDirectory, configuration, communication)

	// Handle ONVIF actions
	go onvif.HandleONVIFActions(configuration, communication)

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
	if subStreamEnabled {
		communication.HandleSubStream <- "stop"
	}

	time.Sleep(time.Second * 3)

	queue.Close()
	queue = nil
	communication.Queue = nil
	if subStreamEnabled {
		//subInfile.Close()
		//subInfile = nil
		//subQueue.Close()
		//subQueue = nil
		communication.SubQueue = nil
	}
	//close(communication.HandleMotion)
	//communication.HandleMotion = nil
	//close(communication.HandleAudio)
	//communication.HandleAudio = nil

	// Waiting for some seconds to make sure everything is properly closed.
	log.Log.Info("RunAgent: waiting 3 seconds to make sure everything is properly closed.")
	time.Sleep(time.Second * 3)
	/*

		if err == nil

			// We might have a secondary rtsp url, so we might need to use that.
			var subInfile av.DemuxCloser
			var subStreams []av.CodecData
			subStreamEnabled := false
			subRtspUrl := config.Capture.IPCamera.SubRTSP
			if subRtspUrl != "" && subRtspUrl != rtspUrl {
				withBackChannel := false
				subInfile, subStreams, err = capture.OpenRTSP(context.Background(), subRtspUrl, withBackChannel) // We'll try to enable backchannel for the substream.
				if err == nil {
					log.Log.Info("RunAgent: opened RTSP sub stream " + subRtspUrl)
					subStreamEnabled = true
				}

				videoStream, _ := capture.GetVideoStream(subStreams)
				if videoStream == nil {
					log.Log.Error("RunAgent: no video substream found, might be the wrong codec (we only support H264 for the moment)")
					time.Sleep(time.Second * 3)
					return status
				}

				width := videoStream.(av.VideoCodecData).Width()
				height := videoStream.(av.VideoCodecData).Height()

				// Set config values as well
				configuration.Config.Capture.IPCamera.Width = width
				configuration.Config.Capture.IPCamera.Height = height
			}

			if cameraSettings.RTSP != rtspUrl || cameraSettings.SubRTSP != subRtspUrl || cameraSettings.Width != width || cameraSettings.Height != height || cameraSettings.Num != num || cameraSettings.Denum != denum || cameraSettings.Codec != videoStream.(av.VideoCodecData).Type() {

				if cameraSettings.RTSP != "" && cameraSettings.SubRTSP != "" && cameraSettings.Initialized {
					decoder.Close()
					if subStreamEnabled {
						subDecoder.Close()
					}
				}

				// At some routines we will need to decode the image.
				// Make sure its properly locked as we only have a single decoder.
				log.Log.Info("RunAgent: camera settings changed, reloading decoder")
				capture.GetVideoDecoder(decoder, streams)
				if subStreamEnabled {
					capture.GetVideoDecoder(subDecoder, subStreams)
				}

				cameraSettings.RTSP = rtspUrl
				cameraSettings.SubRTSP = subRtspUrl
				cameraSettings.Width = width
				cameraSettings.Height = height
				cameraSettings.Framerate = float64(num) / float64(denum)
				cameraSettings.Num = num
				cameraSettings.Denum = denum
				cameraSettings.Codec = videoStream.(av.VideoCodecData).Type()
				cameraSettings.Initialized = true

			} else {
				log.Log.Info("RunAgent: camera settings did not change, keeping decoder")
			}

			communication.Decoder = decoder
			communication.SubDecoder = subDecoder
			communication.DecoderMutex = &decoderMutex
			communication.SubDecoderMutex = &subDecoderMutex

			// Create a packet queue, which is filled by the HandleStream routing
			// and consumed by all other routines: motion, livestream, etc.
			if config.Capture.PreRecording <= 0 {
				config.Capture.PreRecording = 1
				log.Log.Warning("RunAgent: Prerecording value not found in config or invalid value! Found: " + strconv.FormatInt(config.Capture.PreRecording, 10))
			}

			// We are creating a queue to store the RTSP frames in, these frames will be
			// processed by the different consumers: motion detection, recording, etc.
			queue = pubsub.NewQueue()
			communication.Queue = queue
			queue.SetMaxGopCount(int(config.Capture.PreRecording) + 1) // GOP time frame is set to prerecording (we'll add 2 gops to leave some room).
			log.Log.Info("RunAgent: SetMaxGopCount was set with: " + strconv.Itoa(int(config.Capture.PreRecording)+1))
			queue.WriteHeader(streams)

			// We might have a substream, if so we'll create a seperate queue.
			if subStreamEnabled {
				log.Log.Info("RunAgent: Creating sub stream queue with SetMaxGopCount set to " + strconv.Itoa(int(1)))
				subQueue = pubsub.NewQueue()
				communication.SubQueue = subQueue
				subQueue.SetMaxGopCount(1)
				subQueue.WriteHeader(subStreams)
			}

			// Handle the camera stream
			go capture.HandleStream(infile, queue, communication)

			// Handle the substream if enabled
			if subStreamEnabled {
				go capture.HandleSubStream(subInfile, subQueue, communication)
			}

			// Handle processing of audio
			communication.HandleAudio = make(chan models.AudioDataPartial)

			// Handle processing of motion
			communication.HandleMotion = make(chan models.MotionDataPartial, 1)
			if subStreamEnabled {
				motionCursor := subQueue.Latest()
				go computervision.ProcessMotion(motionCursor, configuration, communication, mqttClient, subDecoder, &subDecoderMutex)
			} else {
				motionCursor := queue.Latest()
				go computervision.ProcessMotion(motionCursor, configuration, communication, mqttClient, decoder, &decoderMutex)
			}

			// Handle livestream SD (low resolution over MQTT)
			if subStreamEnabled {
				livestreamCursor := subQueue.Latest()
				go cloud.HandleLiveStreamSD(livestreamCursor, configuration, communication, mqttClient, subDecoder, &subDecoderMutex)
			} else {
				livestreamCursor := queue.Latest()
				go cloud.HandleLiveStreamSD(livestreamCursor, configuration, communication, mqttClient, decoder, &decoderMutex)
			}

			// Handle livestream HD (high resolution over WEBRTC)
			communication.HandleLiveHDHandshake = make(chan models.RequestHDStreamPayload, 1)
			if subStreamEnabled {
				livestreamHDCursor := subQueue.Latest()
				go cloud.HandleLiveStreamHD(livestreamHDCursor, configuration, communication, mqttClient, subStreams, subDecoder, &decoderMutex)
			} else {
				livestreamHDCursor := queue.Latest()
				go cloud.HandleLiveStreamHD(livestreamHDCursor, configuration, communication, mqttClient, streams, decoder, &decoderMutex)
			}

			// Handle recording, will write an mp4 to disk.
			go capture.HandleRecordStream(queue, configDirectory, configuration, communication, streams)

			// Handle Upload to cloud provider (Kerberos Hub, Kerberos Vault and others)
			go cloud.HandleUpload(configDirectory, configuration, communication)

			// Handle ONVIF actions
			go onvif.HandleONVIFActions(configuration, communication)

			// If we reach this point, we have a working RTSP connection.
			communication.CameraConnected = true

			// We might have a camera with audio backchannel enabled.
			// Check if we have a stream with a backchannel and is PCMU encoded.
			go WriteAudioToBackchannel(infile, streams, communication)

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
			if subStreamEnabled {
				communication.HandleSubStream <- "stop"
			}

			time.Sleep(time.Second * 3)

			infile.Close()
			infile = nil
			queue.Close()
			queue = nil
			communication.Queue = nil
			if subStreamEnabled {
				subInfile.Close()
				subInfile = nil
				subQueue.Close()
				subQueue = nil
				communication.SubQueue = nil
			}
			close(communication.HandleMotion)
			communication.HandleMotion = nil
			close(communication.HandleAudio)
			communication.HandleAudio = nil

			// Waiting for some seconds to make sure everything is properly closed.
			log.Log.Info("RunAgent: waiting 3 seconds to make sure everything is properly closed.")
			time.Sleep(time.Second * 3)

		} else {
			log.Log.Error("Something went wrong while opening RTSP: " + err.Error())
			time.Sleep(time.Second * 3)
		}

		log.Log.Debug("RunAgent: finished")

		// Clean up, force garbage collection
		runtime.GC()*/

	// Close the connection to the RTSP server.
	err = rtspClient.Close()
	if err != nil {
		log.Log.Error("RunAgent: error closing RTSP stream: " + err.Error())
		time.Sleep(time.Second * 3)
		return status
	}

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
