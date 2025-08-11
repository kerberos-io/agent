package components

import (
	"context"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"

	"github.com/kerberos-io/agent/machinery/src/capture"
	"github.com/kerberos-io/agent/machinery/src/cloud"
	"github.com/kerberos-io/agent/machinery/src/computervision"
	configService "github.com/kerberos-io/agent/machinery/src/config"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/onvif"
	"github.com/kerberos-io/agent/machinery/src/packets"
	routers "github.com/kerberos-io/agent/machinery/src/routers/mqtt"
	"github.com/kerberos-io/agent/machinery/src/utils"
	"github.com/tevino/abool"
)

var tracer = otel.Tracer("github.com/kerberos-io/agent/machinery/src/components")

func Bootstrap(ctx context.Context, configDirectory string, configuration *models.Configuration, communication *models.Communication, captureDevice *capture.Capture) {

	log.Log.Debug("components.Kerberos.Bootstrap(): bootstrapping the kerberos agent.")

	bootstrapContext := context.Background()
	_, span := tracer.Start(bootstrapContext, "Bootstrap")

	// We will keep track of the Kerberos Agent up time
	// This is send to Kerberos Hub in a heartbeat.
	uptimeStart := time.Now()

	// Initiate the packet counter, this is being used to detect
	// if a camera is going blocky, or got disconnected.
	var packageCounter atomic.Value
	packageCounter.Store(int64(0))
	communication.PackageCounter = &packageCounter

	var packageCounterSub atomic.Value
	packageCounterSub.Store(int64(0))
	communication.PackageCounterSub = &packageCounterSub

	// This is used when the last packet was received (timestamp),
	// this metric is used to determine if the camera is still online/connected.
	var lastPacketTimer atomic.Value
	packageCounter.Store(int64(0))
	communication.LastPacketTimer = &lastPacketTimer

	var lastPacketTimerSub atomic.Value
	packageCounterSub.Store(int64(0))
	communication.LastPacketTimerSub = &lastPacketTimerSub

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

	span.End()

	// Run the agent and fire up all the other
	// goroutines which do image capture, motion detection, onvif, etc.
	for {

		// This will blocking until receiving a signal to be restarted, reconfigured, stopped, etc.
		status := RunAgent(configDirectory, configuration, communication, mqttClient, uptimeStart, cameraSettings, captureDevice)

		if status == "stop" {
			log.Log.Info("components.Kerberos.Bootstrap(): shutting down the agent in 3 seconds.")
			time.Sleep(time.Second * 3)
			os.Exit(0)
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
}

func RunAgent(configDirectory string, configuration *models.Configuration, communication *models.Communication, mqttClient mqtt.Client, uptimeStart time.Time, cameraSettings *models.Camera, captureDevice *capture.Capture) string {

	ctx := context.Background()
	ctxRunAgent, span := tracer.Start(ctx, "RunAgent")

	log.Log.Info("components.Kerberos.RunAgent(): Creating camera and processing threads.")
	config := configuration.Config

	status := "not started"

	// Currently only support H264 encoded cameras, this will change.
	// Establishing the camera connection without backchannel if no substream
	rtspUrl := config.Capture.IPCamera.RTSP
	rtspClient := captureDevice.SetMainClient(rtspUrl)
	if rtspUrl != "" {
		err := rtspClient.Connect(ctx, ctxRunAgent)
		if err != nil {
			log.Log.Error("components.Kerberos.RunAgent(): error connecting to RTSP stream: " + err.Error())
			rtspClient.Close(ctxRunAgent)
			rtspClient = nil
			time.Sleep(time.Second * 3)
			return status
		}
	} else {
		log.Log.Error("components.Kerberos.RunAgent(): no rtsp url found in config, please provide one.")
		rtspClient = nil
		time.Sleep(time.Second * 3)
		return status
	}

	log.Log.Info("components.Kerberos.RunAgent(): opened RTSP stream: " + rtspUrl)

	// Get the video streams from the RTSP server.
	videoStreams, err := rtspClient.GetVideoStreams()
	if err != nil || len(videoStreams) == 0 {
		log.Log.Error("components.Kerberos.RunAgent(): no video stream found, might be the wrong codec (we only support H264 for the moment)")
		rtspClient.Close(ctxRunAgent)
		time.Sleep(time.Second * 3)
		return status
	}

	// Get the video stream from the RTSP server.
	videoStream := videoStreams[0]

	// Get some information from the video stream.
	width := videoStream.Width
	height := videoStream.Height

	// Set config values as well
	configuration.Config.Capture.IPCamera.Width = width
	configuration.Config.Capture.IPCamera.Height = height

	// Set the liveview width and height, this is used for the liveview and motion regions (drawing on the hub).
	baseWidth := config.Capture.IPCamera.BaseWidth
	baseHeight := config.Capture.IPCamera.BaseHeight
	// If the liveview height is not set, we will calculate it based on the width and aspect ratio of the camera.
	if baseWidth > 0 && baseHeight == 0 {
		widthAspectRatio := float64(baseWidth) / float64(width)
		configuration.Config.Capture.IPCamera.BaseHeight = int(float64(height) * widthAspectRatio)
	} else if baseHeight > 0 && baseWidth > 0 {
		configuration.Config.Capture.IPCamera.BaseHeight = baseHeight
		configuration.Config.Capture.IPCamera.BaseWidth = baseWidth
	} else {
		configuration.Config.Capture.IPCamera.BaseHeight = height
		configuration.Config.Capture.IPCamera.BaseWidth = width
	}

	// Set the SPS and PPS values in the configuration.
	configuration.Config.Capture.IPCamera.SPSNALUs = [][]byte{videoStream.SPS}
	configuration.Config.Capture.IPCamera.PPSNALUs = [][]byte{videoStream.PPS}
	configuration.Config.Capture.IPCamera.VPSNALUs = [][]byte{videoStream.VPS}

	// Define queues for the main and sub stream.
	var queue *packets.Queue
	var subQueue *packets.Queue

	// Create a packet queue, which is filled by the HandleStream routing
	// and consumed by all other routines: motion, livestream, etc.
	if config.Capture.PreRecording <= 0 {
		config.Capture.PreRecording = 1
		log.Log.Warning("components.Kerberos.RunAgent(): Prerecording value not found in config or invalid value! Found: " + strconv.FormatInt(config.Capture.PreRecording, 10))
	}

	// We might have a secondary rtsp url, so we might need to use that for livestreaming let us check first!
	subStreamEnabled := false
	subRtspUrl := config.Capture.IPCamera.SubRTSP
	var videoSubStreams []packets.Stream

	if subRtspUrl != "" && subRtspUrl != rtspUrl {
		// For the sub stream we will not enable backchannel.
		subStreamEnabled = true
		rtspSubClient := captureDevice.SetSubClient(subRtspUrl)
		captureDevice.RTSPSubClient = rtspSubClient

		err := rtspSubClient.Connect(ctx, ctxRunAgent)
		if err != nil {
			log.Log.Error("components.Kerberos.RunAgent(): error connecting to RTSP sub stream: " + err.Error())
			time.Sleep(time.Second * 3)
			return status
		}
		log.Log.Info("components.Kerberos.RunAgent(): opened RTSP sub stream: " + subRtspUrl)

		// Get the video streams from the RTSP server.
		videoSubStreams, err = rtspSubClient.GetVideoStreams()
		if err != nil || len(videoSubStreams) == 0 {
			log.Log.Error("components.Kerberos.RunAgent(): no video sub stream found, might be the wrong codec (we only support H264 for the moment)")
			rtspSubClient.Close(ctxRunAgent)
			time.Sleep(time.Second * 3)
			return status
		}

		// Get the video stream from the RTSP server.
		videoSubStream := videoSubStreams[0]

		width := videoSubStream.Width
		height := videoSubStream.Height

		// Set config values as well
		configuration.Config.Capture.IPCamera.SubWidth = width
		configuration.Config.Capture.IPCamera.SubHeight = height

		// If we have a substream, we need to set the width and height of the substream. (so we will override above information)
		// Set the liveview width and height, this is used for the liveview and motion regions (drawing on the hub).
		baseWidth := config.Capture.IPCamera.BaseWidth
		baseHeight := config.Capture.IPCamera.BaseHeight
		// If the liveview height is not set, we will calculate it based on the width and aspect ratio of the camera.
		if baseWidth > 0 && baseHeight == 0 {
			widthAspectRatio := float64(baseWidth) / float64(width)
			configuration.Config.Capture.IPCamera.BaseHeight = int(float64(height) * widthAspectRatio)
		} else if baseHeight > 0 && baseWidth > 0 {
			configuration.Config.Capture.IPCamera.BaseHeight = baseHeight
			configuration.Config.Capture.IPCamera.BaseWidth = baseWidth
		} else {
			configuration.Config.Capture.IPCamera.BaseHeight = height
			configuration.Config.Capture.IPCamera.BaseWidth = width
		}
	}

	// We are creating a queue to store the RTSP frames in, these frames will be
	// processed by the different consumers: motion detection, recording, etc.
	queue = packets.NewQueue()
	communication.Queue = queue

	// Set the maximum GOP count, this is used to determine the pre-recording time.
	log.Log.Info("components.Kerberos.RunAgent(): SetMaxGopCount was set with: " + strconv.Itoa(int(config.Capture.PreRecording)+1))
	queue.SetMaxGopCount(1) // We will adjust this later on, when we have the GOP size.
	queue.WriteHeader(videoStreams)
	go rtspClient.Start(ctx, "main", queue, configuration, communication)

	// Main stream is connected and ready to go.
	communication.MainStreamConnected = true

	// Try to create backchannel
	rtspBackChannelClient := captureDevice.SetBackChannelClient(rtspUrl)
	err = rtspBackChannelClient.ConnectBackChannel(ctx, ctxRunAgent)
	if err == nil {
		log.Log.Info("components.Kerberos.RunAgent(): opened RTSP backchannel stream: " + rtspUrl)
		go rtspBackChannelClient.StartBackChannel(ctx, ctxRunAgent)
	}

	rtspSubClient := captureDevice.RTSPSubClient
	if subStreamEnabled && rtspSubClient != nil {
		subQueue = packets.NewQueue()
		communication.SubQueue = subQueue
		subQueue.SetMaxGopCount(1) // GOP time frame is set to 1 for motion detection and livestreaming.
		subQueue.WriteHeader(videoSubStreams)
		go rtspSubClient.Start(ctx, "sub", subQueue, configuration, communication)

		// Sub stream is connected and ready to go.
		communication.SubStreamConnected = true
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
	communication.HandleLiveHDHandshake = make(chan models.RequestHDStreamPayload, 10)
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
	communication.HandleMotion = make(chan models.MotionDataPartial, 10)
	if subStreamEnabled {
		motionCursor := subQueue.Latest()
		go computervision.ProcessMotion(motionCursor, configuration, communication, mqttClient, rtspSubClient)
	} else {
		motionCursor := queue.Latest()
		go computervision.ProcessMotion(motionCursor, configuration, communication, mqttClient, rtspClient)
	}

	// Handle realtime processing if enabled.
	if subStreamEnabled {
		realtimeProcessingCursor := subQueue.Latest()
		go cloud.HandleRealtimeProcessing(realtimeProcessingCursor, configuration, communication, mqttClient, rtspClient)
	} else {
		realtimeProcessingCursor := queue.Latest()
		go cloud.HandleRealtimeProcessing(realtimeProcessingCursor, configuration, communication, mqttClient, rtspClient)
	}

	// Handle Upload to cloud provider (Kerberos Hub, Kerberos Vault and others)
	go cloud.HandleUpload(configDirectory, configuration, communication)

	// Handle ONVIF actions
	communication.HandleONVIF = make(chan models.OnvifAction, 10)
	go onvif.HandleONVIFActions(configuration, communication)

	communication.HandleAudio = make(chan models.AudioDataPartial, 10)
	if rtspBackChannelClient.HasBackChannel {
		communication.HasBackChannel = true
		go WriteAudioToBackchannel(communication, rtspBackChannelClient)
	}

	// If we reach this point, we have a working RTSP connection.
	communication.CameraConnected = true

	// Otel end span
	span.End()

	// !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
	// This will go into a blocking state, once this channel is triggered
	// the agent will cleanup and restart.

	status = <-communication.HandleBootstrap

	// If we reach this point, we are stopping the stream.
	communication.CameraConnected = false
	communication.MainStreamConnected = false
	communication.SubStreamConnected = false

	// Cancel the main context, this will stop all the other goroutines.
	(*communication.CancelContext)()

	// We will re open the configuration, might have changed :O!
	configService.OpenConfig(configDirectory, configuration)

	// We will override the configuration with the environment variables
	configService.OverrideWithEnvironmentVabriables(configuration)

	// Here we are cleaning up everything!
	if configuration.Config.Offline != "true" {
		select {
		case communication.HandleUpload <- "stop":
			log.Log.Info("components.Kerberos.RunAgent(): stopping upload")
		case <-time.After(1 * time.Second):
			log.Log.Info("components.Kerberos.RunAgent(): stopping upload timed out")
		}
	}

	select {
	case communication.HandleStream <- "stop":
		log.Log.Info("components.Kerberos.RunAgent(): stopping stream")
	case <-time.After(1 * time.Second):
		log.Log.Info("components.Kerberos.RunAgent(): stopping stream timed out")
	}
	// We use the steam channel to stop both main and sub stream.
	//if subStreamEnabled {
	//	communication.HandleSubStream <- "stop"
	//}

	time.Sleep(time.Second * 3)

	err = rtspClient.Close(ctxRunAgent)
	if err != nil {
		log.Log.Error("components.Kerberos.RunAgent(): error closing RTSP stream: " + err.Error())
		time.Sleep(time.Second * 3)
		return status
	}

	queue.Close()
	queue = nil
	communication.Queue = nil

	if subStreamEnabled {
		err = rtspSubClient.Close(ctxRunAgent)
		if err != nil {
			log.Log.Error("components.Kerberos.RunAgent(): error closing RTSP sub stream: " + err.Error())
			time.Sleep(time.Second * 3)
			return status
		}
		subQueue.Close()
		subQueue = nil
		communication.SubQueue = nil
	}

	err = rtspBackChannelClient.Close(ctxRunAgent)
	if err != nil {
		log.Log.Error("components.Kerberos.RunAgent(): error closing RTSP backchannel stream: " + err.Error())
	}

	time.Sleep(time.Second * 3)

	close(communication.HandleLiveHDHandshake)
	communication.HandleLiveHDHandshake = nil

	close(communication.HandleMotion)
	communication.HandleMotion = nil

	close(communication.HandleAudio)
	communication.HandleAudio = nil

	close(communication.HandleONVIF)
	communication.HandleONVIF = nil

	// Waiting for some seconds to make sure everything is properly closed.
	log.Log.Info("components.Kerberos.RunAgent(): waiting 3 seconds to make sure everything is properly closed.")
	time.Sleep(time.Second * 3)

	return status
}

// ControlAgent will check if the camera is still connected, if not it will restart the agent.
// In the other thread we are keeping track of the number of packets received, and particular the keyframe packets.
// Once we are not receiving any packets anymore, we will restart the agent.
func ControlAgent(communication *models.Communication) {
	log.Log.Debug("components.Kerberos.ControlAgent(): started")
	packageCounter := communication.PackageCounter
	packageSubCounter := communication.PackageCounterSub
	go func() {
		// A channel to check the camera activity
		var previousPacket int64 = 0
		var previousPacketSub int64 = 0
		var occurence = 0
		var occurenceSub = 0
		for {

			// If camera is connected, we'll check if we are still receiving packets.
			if communication.CameraConnected {

				// First we'll check the main stream.
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

				log.Log.Info("components.Kerberos.ControlAgent(): Number of packets read from mainstream: " + strconv.FormatInt(packetsR, 10))

				// After 15 seconds without activity this is thrown..
				if occurence == 3 {
					log.Log.Info("components.Kerberos.ControlAgent(): Restarting machinery because of blocking mainstream.")
					select {
					case communication.HandleBootstrap <- "restart":
						log.Log.Info("components.Kerberos.ControlAgent(): Restarting machinery because of blocking substream.")
					case <-time.After(1 * time.Second):
						log.Log.Info("components.Kerberos.ControlAgent(): Restarting machinery because of blocking substream timed out")
					}
					occurence = 0
				}

				// Now we'll check the sub stream.
				packetsSubR := packageSubCounter.Load().(int64)
				if communication.SubStreamConnected {
					if packetsSubR == previousPacketSub {
						// If we are already reconfiguring,
						// we dont need to check if the stream is blocking.
						if !communication.IsConfiguring.IsSet() {
							occurenceSub = occurenceSub + 1
						}
					} else {
						occurenceSub = 0
					}

					log.Log.Info("components.Kerberos.ControlAgent(): Number of packets read from substream: " + strconv.FormatInt(packetsSubR, 10))

					// After 15 seconds without activity this is thrown..
					if occurenceSub == 3 {
						select {
						case communication.HandleBootstrap <- "restart":
							log.Log.Info("components.Kerberos.ControlAgent(): Restarting machinery because of blocking substream.")
						case <-time.After(1 * time.Second):
							log.Log.Info("components.Kerberos.ControlAgent(): Restarting machinery because of blocking substream timed out")
						}
						occurenceSub = 0
					}
				}

				previousPacket = packageCounter.Load().(int64)
				previousPacketSub = packageSubCounter.Load().(int64)
			}

			time.Sleep(5 * time.Second)
		}
	}()
	log.Log.Debug("components.Kerberos.ControlAgent(): finished")
}

// GetDashboard godoc
// @Router /api/dashboard [get]
// @ID dashboard
// @Tags general
// @Summary Get all information showed on the dashboard.
// @Description Get all information showed on the dashboard.
// @Success 200
func GetDashboard(c *gin.Context, configDirectory string, configuration *models.Configuration, communication *models.Communication) {

	// Check if camera is online.
	cameraIsOnline := communication.CameraConnected

	// If an agent is properly setup with Kerberos Hub, we will send
	// a ping to Kerberos Hub every 15seconds. On receiving a positive response
	// it will update the CloudTimestamp value.
	cloudIsOnline := false
	if communication.CloudTimestamp != nil && communication.CloudTimestamp.Load() != nil {
		timestamp := communication.CloudTimestamp.Load().(int64)
		if timestamp > 0 {
			cloudIsOnline = true
		}
	}

	// The total number of recordings stored in the directory.
	recordingDirectory := configDirectory + "/data/recordings"
	numberOfRecordings := utils.NumberOfMP4sInDirectory(recordingDirectory)

	// All days stored in this agent.
	days := []string{}
	latestEvents := []models.Media{}
	files, err := utils.ReadDirectory(recordingDirectory)
	if err == nil {
		events := utils.GetSortedDirectory(files)

		// Get All days
		days = utils.GetDays(events, recordingDirectory, configuration)

		// Get all latest events
		var eventFilter models.EventFilter
		eventFilter.NumberOfElements = 5
		latestEvents = utils.GetMediaFormatted(events, recordingDirectory, configuration, eventFilter) // will get 5 latest recordings.
	}

	c.JSON(200, gin.H{
		"offlineMode":        configuration.Config.Offline,
		"cameraOnline":       cameraIsOnline,
		"cloudOnline":        cloudIsOnline,
		"numberOfRecordings": numberOfRecordings,
		"days":               days,
		"latestEvents":       latestEvents,
	})
}

// GetLatestEvents godoc
// @Router /api/latest-events [post]
// @ID latest-events
// @Tags general
// @Param eventFilter body models.EventFilter true "Event filter"
// @Summary Get the latest recordings (events) from the recordings directory.
// @Description Get the latest recordings (events) from the recordings directory.
// @Success 200
func GetLatestEvents(c *gin.Context, configDirectory string, configuration *models.Configuration, communication *models.Communication) {
	var eventFilter models.EventFilter
	err := c.BindJSON(&eventFilter)
	if err == nil {
		// Default to 10 if no limit is set.
		if eventFilter.NumberOfElements == 0 {
			eventFilter.NumberOfElements = 10
		}
		recordingDirectory := configDirectory + "/data/recordings"
		files, err := utils.ReadDirectory(recordingDirectory)
		if err == nil {
			events := utils.GetSortedDirectory(files)
			// We will get all recordings from the directory (as defined by the filter).
			fileObjects := utils.GetMediaFormatted(events, recordingDirectory, configuration, eventFilter)
			c.JSON(200, gin.H{
				"events": fileObjects,
			})
		} else {
			c.JSON(400, gin.H{
				"data": "Something went wrong: " + err.Error(),
			})
		}
	} else {
		c.JSON(400, gin.H{
			"data": "Something went wrong: " + err.Error(),
		})
	}
}

// GetDays godoc
// @Router /api/days [get]
// @ID days
// @Tags general
// @Summary Get all days stored in the recordings directory.
// @Description Get all days stored in the recordings directory.
// @Success 200
func GetDays(c *gin.Context, configDirectory string, configuration *models.Configuration, communication *models.Communication) {
	recordingDirectory := configDirectory + "/data/recordings"
	files, err := utils.ReadDirectory(recordingDirectory)
	if err == nil {
		events := utils.GetSortedDirectory(files)
		days := utils.GetDays(events, recordingDirectory, configuration)
		c.JSON(200, gin.H{
			"events": days,
		})
	} else {
		c.JSON(400, gin.H{
			"data": "Something went wrong: " + err.Error(),
		})
	}
}

// StopAgent godoc
// @Router /api/camera/stop [post]
// @ID camera-stop
// @Tags camera
// @Summary Stop the agent.
// @Description Stop the agent.
// @Success 200 {object} models.APIResponse
func StopAgent(c *gin.Context, communication *models.Communication) {
	log.Log.Info("components.Kerberos.StopAgent(): sending signal to stop agent, this will os.Exit(0).")
	select {
	case communication.HandleBootstrap <- "stop":
		log.Log.Info("components.Kerberos.StopAgent(): Stopping machinery.")
	case <-time.After(1 * time.Second):
		log.Log.Info("components.Kerberos.StopAgent(): Stopping machinery timed out")
	}
	c.JSON(200, gin.H{
		"stopped": true,
	})
}

// RestartAgent godoc
// @Router /api/camera/restart [post]
// @ID camera-restart
// @Tags camera
// @Summary Restart the agent.
// @Description Restart the agent.
// @Success 200 {object} models.APIResponse
func RestartAgent(c *gin.Context, communication *models.Communication) {
	log.Log.Info("components.Kerberos.RestartAgent(): sending signal to restart agent.")
	select {
	case communication.HandleBootstrap <- "restart":
		log.Log.Info("components.Kerberos.RestartAgent(): Restarting machinery.")
	case <-time.After(1 * time.Second):
		log.Log.Info("components.Kerberos.RestartAgent(): Restarting machinery timed out")
	}
	c.JSON(200, gin.H{
		"restarted": true,
	})
}

// MakeRecording godoc
// @Router /api/camera/record [post]
// @ID camera-record
// @Tags camera
// @Summary Make a recording.
// @Description Make a recording.
// @Success 200 {object} models.APIResponse
func MakeRecording(c *gin.Context, communication *models.Communication) {
	log.Log.Info("components.Kerberos.MakeRecording(): sending signal to start recording.")
	dataToPass := models.MotionDataPartial{
		Timestamp:       time.Now().Unix(),
		NumberOfChanges: 100000000, // hack set the number of changes to a high number to force recording
	}
	communication.HandleMotion <- dataToPass //Save data to the channel
	c.JSON(200, gin.H{
		"recording": true,
	})
}

// GetSnapshotBase64 godoc
// @Router /api/camera/snapshot/base64 [get]
// @ID snapshot-base64
// @Tags camera
// @Summary Get a snapshot from the camera in base64.
// @Description Get a snapshot from the camera in base64.
// @Success 200
func GetSnapshotBase64(c *gin.Context, captureDevice *capture.Capture, configuration *models.Configuration, communication *models.Communication) {
	// We'll try to get a snapshot from the camera.
	base64Image := capture.Base64Image(captureDevice, communication, configuration)
	if base64Image != "" {
		communication.Image = base64Image
	}

	c.JSON(200, gin.H{
		"base64": communication.Image,
	})
}

// GetSnapshotJpeg godoc
// @Router /api/camera/snapshot/jpeg [get]
// @ID snapshot-jpeg
// @Tags camera
// @Summary Get a snapshot from the camera in jpeg format.
// @Description Get a snapshot from the camera in jpeg format.
// @Success 200
func GetSnapshotRaw(c *gin.Context, captureDevice *capture.Capture, configuration *models.Configuration, communication *models.Communication) {
	// We'll try to get a snapshot from the camera.
	image := capture.JpegImage(captureDevice, communication)

	// encode image to jpeg
	imageResized, _ := utils.ResizeImage(&image, uint(configuration.Config.Capture.IPCamera.BaseWidth), uint(configuration.Config.Capture.IPCamera.BaseHeight))
	bytes, _ := utils.ImageToBytes(imageResized)

	// Return image/jpeg
	c.Data(200, "image/jpeg", bytes)
}

// GetConfig godoc
// @Router /api/config [get]
// @ID config
// @Tags config
// @Summary Get the current configuration.
// @Description Get the current configuration.
// @Success 200
func GetConfig(c *gin.Context, captureDevice *capture.Capture, configuration *models.Configuration, communication *models.Communication) {
	// We'll try to get a snapshot from the camera.
	base64Image := capture.Base64Image(captureDevice, communication, configuration)
	if base64Image != "" {
		communication.Image = base64Image
	}

	c.JSON(200, gin.H{
		"config":   configuration.Config,
		"custom":   configuration.CustomConfig,
		"global":   configuration.GlobalConfig,
		"snapshot": communication.Image,
	})
}

// UpdateConfig godoc
// @Router /api/config [post]
// @ID config
// @Tags config
// @Param config body models.Config true "Configuration"
// @Summary Update the current configuration.
// @Description Update the current configuration.
// @Success 200
func UpdateConfig(c *gin.Context, configDirectory string, configuration *models.Configuration, communication *models.Communication) {
	var config models.Config
	err := c.BindJSON(&config)
	if err == nil {
		err := configService.SaveConfig(configDirectory, config, configuration, communication)
		if err == nil {
			c.JSON(200, gin.H{
				"data": "☄ Reconfiguring",
			})
		} else {
			c.JSON(200, gin.H{
				"data": "☄ Reconfiguring",
			})
		}
	} else {
		c.JSON(400, gin.H{
			"data": "Something went wrong: " + err.Error(),
		})
	}
}
