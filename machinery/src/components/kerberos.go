package components

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
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
	"github.com/kerberos-io/agent/machinery/src/webrtc"
	"github.com/tevino/abool"
)

var tracer = otel.Tracer("github.com/kerberos-io/agent/machinery/src/components")

type runWorkers struct {
	waitGroup sync.WaitGroup
}

func (w *runWorkers) Start(worker func()) {
	w.waitGroup.Add(1)
	go func() {
		defer w.waitGroup.Done()
		worker()
	}()
}

func (w *runWorkers) Wait(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		w.waitGroup.Wait()
		close(done)
	}()

	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

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
	lastPacketTimer.Store(int64(0))
	communication.LastPacketTimer = &lastPacketTimer

	var lastPacketTimerSub atomic.Value
	lastPacketTimerSub.Store(int64(0))
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
	communication.HandleLiveSDHTTP = make(chan int64, 1)
	communication.HandleLiveHDKeepalive = make(chan string, 1)
	communication.HandleLiveHDPeers = make(chan string, 1)
	communication.HandleLiveHLS = make(chan string, 1)
	communication.HandleAudio = make(chan models.AudioDataPartial, 10)
	communication.IsConfiguring = abool.New()
	communication.IsRecordingManual = abool.New()
	communication.RecordingManualHeartbeat = &atomic.Int64{}
	communication.RecordingManualStart = &atomic.Int64{}
	communication.RecordingManualHeartbeatSeen = abool.New()

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
	workers := &runWorkers{}
	var rtspSubClient *capture.Golibrtsp
	mainClientNeedsClose := false
	subClientNeedsClose := false

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
		mainClientNeedsClose = true
	} else {
		log.Log.Error("components.Kerberos.RunAgent(): no rtsp url found in config, please provide one.")
		rtspClient = nil
		time.Sleep(time.Second * 3)
		return status
	}
	defer func() {
		if subClientNeedsClose && rtspSubClient != nil {
			if closeErr := rtspSubClient.Close(ctxRunAgent); closeErr != nil {
				log.Log.Error("components.Kerberos.RunAgent(): error closing RTSP sub stream after partial startup: " + closeErr.Error())
			}
		}
		if mainClientNeedsClose && rtspClient != nil {
			if closeErr := rtspClient.Close(ctxRunAgent); closeErr != nil {
				log.Log.Error("components.Kerberos.RunAgent(): error closing RTSP stream after partial startup: " + closeErr.Error())
			}
		}
	}()

	log.Log.Info("components.Kerberos.RunAgent(): opened RTSP stream: " + rtspUrl)

	// Get the video streams from the RTSP server.
	videoStreams, err := rtspClient.GetVideoStreams()
	if err != nil || len(videoStreams) == 0 {
		log.Log.Error("components.Kerberos.RunAgent(): no video stream found, might be the wrong codec (we only support H264 for the moment)")
		time.Sleep(time.Second * 3)
		return status
	}

	// Get the video stream from the RTSP server.
	videoStream := videoStreams[0]
	log.Log.Info(fmt.Sprintf("components.Kerberos.RunAgent(): detected main video stream: codec=%s resolution=%dx%d fps=%.2f", videoStream.Name, videoStream.Width, videoStream.Height, videoStream.FPS))

	// Get some information from the video stream.
	width := videoStream.Width
	height := videoStream.Height

	// Set config values as well
	configuration.Config.Capture.IPCamera.Width = width
	configuration.Config.Capture.IPCamera.Height = height

	// Set the liveview width and height, this is used for the liveview and motion regions (drawing on the hub).
	// ResolveBaseDimensions gates the aspect-ratio compute on width/height > 0
	// so a not-yet-probed stream can't poison the dimensions and crash resize.
	configuration.Config.Capture.IPCamera.BaseWidth, configuration.Config.Capture.IPCamera.BaseHeight =
		utils.ResolveBaseDimensions(config.Capture.IPCamera.BaseWidth, config.Capture.IPCamera.BaseHeight, width, height)

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
		rtspSubClient = captureDevice.SetSubClient(subRtspUrl)
		subClientNeedsClose = true

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
			time.Sleep(time.Second * 3)
			return status
		}

		// Get the video stream from the RTSP server.
		videoSubStream := videoSubStreams[0]
		log.Log.Info(fmt.Sprintf("components.Kerberos.RunAgent(): detected sub video stream: codec=%s resolution=%dx%d fps=%.2f", videoSubStream.Name, videoSubStream.Width, videoSubStream.Height, videoSubStream.FPS))

		width := videoSubStream.Width
		height := videoSubStream.Height

		// Set config values as well
		configuration.Config.Capture.IPCamera.SubWidth = width
		configuration.Config.Capture.IPCamera.SubHeight = height

		// Capture the sub stream parameter sets separately from the main stream so
		// the live HLS muxer can build a correct init segment when a viewer asks for
		// the sub (low-resolution) stream on demand.
		configuration.Config.Capture.IPCamera.SubSPSNALUs = [][]byte{videoSubStream.SPS}
		configuration.Config.Capture.IPCamera.SubPPSNALUs = [][]byte{videoSubStream.PPS}
		configuration.Config.Capture.IPCamera.SubVPSNALUs = [][]byte{videoSubStream.VPS}

		// If we have a substream, we need to set the width and height of the substream. (so we will override above information)
		// Set the liveview width and height, this is used for the liveview and motion regions (drawing on the hub).
		configuration.Config.Capture.IPCamera.BaseWidth, configuration.Config.Capture.IPCamera.BaseHeight =
			utils.ResolveBaseDimensions(config.Capture.IPCamera.BaseWidth, config.Capture.IPCamera.BaseHeight, width, height)
	}

	// We are creating a queue to store the RTSP frames in, these frames will be
	// processed by the different consumers: motion detection, recording, etc.
	queue = packets.NewQueue()
	communication.Queue.Store(queue)

	// Set the maximum GOP count, this is used to determine the pre-recording time.
	log.Log.Info("components.Kerberos.RunAgent(): SetMaxGopCount was set with: " + strconv.Itoa(int(config.Capture.PreRecording)+1))
	queue.SetMaxGopCount(1) // We will adjust this later on, when we have the GOP size.
	queue.WriteHeader(videoStreams)
	workers.Start(func() {
		rtspClient.Start(ctx, "main", queue, configuration, communication)
	})

	// Main stream is connected and ready to go.
	communication.MainStreamConnected.Store(true)

	// Try to create backchannel
	communication.HasBackChannel.Store(false)
	rtspBackChannelClient := captureDevice.SetBackChannelClient(rtspUrl)
	err = rtspBackChannelClient.ConnectBackChannel(ctx, ctxRunAgent)
	if err == nil {
		log.Log.Info("components.Kerberos.RunAgent(): opened RTSP backchannel stream: " + rtspUrl)
	}

	if subStreamEnabled && rtspSubClient != nil {
		subQueue = packets.NewQueue()
		communication.SubQueue.Store(subQueue)
		subQueue.SetMaxGopCount(1) // GOP time frame is set to 1 for motion detection and livestreaming.
		subQueue.WriteHeader(videoSubStreams)
		workers.Start(func() {
			rtspSubClient.Start(ctx, "sub", subQueue, configuration, communication)
		})

		// Sub stream is connected and ready to go.
		communication.SubStreamConnected.Store(true)
	}

	// Handle livestream SD (low resolution over MQTT)
	if subStreamEnabled {
		livestreamCursor := subQueue.Latest()
		workers.Start(func() {
			cloud.HandleLiveStreamSD(livestreamCursor, configuration, communication, mqttClient, rtspSubClient)
		})
	} else {
		livestreamCursor := queue.Latest()
		workers.Start(func() {
			cloud.HandleLiveStreamSD(livestreamCursor, configuration, communication, mqttClient, rtspClient)
		})
	}

	// Handle livestream HLS (adaptive segments over HTTP via hub-api -> vault).
	// The producer can serve either the main (high-resolution) or sub
	// (low-resolution) stream and switches between them on demand based on the
	// quality the viewer requests; "auto" prefers the sub stream when available.
	// Like SD it is viewer-keepalive gated and produces no traffic while nobody is
	// watching.
	workers.Start(func() {
		cloud.HandleLiveStreamHLS(configuration, communication, mqttClient, subStreamEnabled)
	})

	// MoQ is available only in the dedicated CGO/glibc build. The standard
	// static Alpine build resolves this hook to a no-op.
	workers.Start(func() {
		cloud.StartLiveStreamMoQ(configuration, communication, subStreamEnabled)
	})

	// Handle livestream HD (high resolution over WEBRTC). Both the main and sub
	// stream are exposed as separate broadcasters so a viewer can request the
	// high (main) or low (sub) resolution per peer connection; "auto" prefers the
	// sub stream when available.
	liveHDHandshakes := make(chan models.LiveHDHandshake, 100)
	motionEvents := make(chan models.MotionDataPartial, 10)
	onvifActions := make(chan models.OnvifAction, 10)
	communication.SetRunChannels(liveHDHandshakes, motionEvents, onvifActions)
	workers.Start(func() {
		cloud.HandleLiveStreamHD(configuration, communication, mqttClient, rtspClient, rtspSubClient, subStreamEnabled, liveHDHandshakes)
	})

	// Handle recording, will write an mp4 to disk.
	workers.Start(func() {
		capture.HandleRecordStream(queue, configDirectory, configuration, communication, rtspClient, mqttClient, motionEvents)
	})

	// Handle processing of motion
	if subStreamEnabled {
		motionCursor := subQueue.Latest()
		workers.Start(func() {
			computervision.ProcessMotion(motionCursor, configuration, communication, mqttClient, rtspSubClient)
		})
	} else {
		motionCursor := queue.Latest()
		workers.Start(func() {
			computervision.ProcessMotion(motionCursor, configuration, communication, mqttClient, rtspClient)
		})
	}

	// Handle realtime processing if enabled.
	if subStreamEnabled {
		realtimeProcessingCursor := subQueue.Latest()
		workers.Start(func() {
			cloud.HandleRealtimeProcessing(realtimeProcessingCursor, configuration, communication, mqttClient, rtspClient)
		})
	} else {
		realtimeProcessingCursor := queue.Latest()
		workers.Start(func() {
			cloud.HandleRealtimeProcessing(realtimeProcessingCursor, configuration, communication, mqttClient, rtspClient)
		})
	}

	// Handle Upload to cloud provider (Kerberos Hub, Kerberos Vault and others)
	workers.Start(func() {
		cloud.HandleUpload(configDirectory, configuration, communication)
	})

	// Handle ONVIF actions
	workers.Start(func() {
		onvif.HandleONVIFActions(configuration, communication, onvifActions)
	})

	// Handle ONVIF event stream — opt-in via Capture.ONVIFMotion="true".
	// Stops when the agent's shared context is cancelled. The function
	// is a no-op if ONVIFMotion is not enabled.
	workers.Start(func() {
		onvif.HandleONVIFEventStream(*communication.Context, configuration, communication)
	})

	if rtspBackChannelClient.HasBackChannel {
		communication.HasBackChannel.Store(true)
		workers.Start(func() {
			WriteAudioToBackchannel(communication, rtspBackChannelClient)
		})
	}

	// If we reach this point, we have a working RTSP connection.
	communication.CameraConnected.Store(true)

	// Otel end span
	span.End()

	// !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
	// This will go into a blocking state, once this channel is triggered
	// the agent will cleanup and restart.

	status = <-communication.HandleBootstrap

	// If we reach this point, we are stopping the stream.
	communication.CameraConnected.Store(false)
	communication.MainStreamConnected.Store(false)
	communication.SubStreamConnected.Store(false)

	// Cancel the main context, this will stop all the other goroutines.
	(*communication.CancelContext)()

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

	mainClientNeedsClose = false
	err = rtspClient.Close(ctxRunAgent)
	if err != nil {
		log.Log.Error("components.Kerberos.RunAgent(): error closing RTSP stream: " + err.Error())
	}

	queue.Close()
	queue = nil

	if subStreamEnabled {
		subClientNeedsClose = false
		err = rtspSubClient.Close(ctxRunAgent)
		if err != nil {
			log.Log.Error("components.Kerberos.RunAgent(): error closing RTSP sub stream: " + err.Error())
		}
		subQueue.Close()
		subQueue = nil
	}

	err = rtspBackChannelClient.Close(ctxRunAgent)
	if err != nil {
		log.Log.Error("components.Kerberos.RunAgent(): error closing RTSP backchannel stream: " + err.Error())
	}

	communication.CloseRunChannels()

	if workers.Wait(runShutdownTimeout) {
		log.Log.Info("components.Kerberos.RunAgent(): all run workers stopped")
	} else {
		communication.RecordRunWorkerShutdownTimeout()
		log.Log.Error("components.Kerberos.RunAgent(): timed out waiting for run workers to stop")
	}
	communication.Queue.Store(nil)
	communication.SubQueue.Store(nil)

	// Factory reads retry transient database failures, so release runtime
	// resources before reopening configuration.
	configService.OpenConfig(configDirectory, configuration)
	configService.OverrideWithEnvironmentVariables(configuration)

	return status
}

// packetAgeString returns a human readable age (e.g. "12s") since the last
// packet timestamp stored in the given atomic.Value, or "unknown" when no
// packet has been received yet. Used to add context to watchdog restart logs.
func packetAgeString(timer *atomic.Value) string {
	if timer == nil {
		return "unknown"
	}

	// atomic.Value panics on Load() if it was never initialized via Store().
	var v any
	func() {
		defer func() {
			if recover() != nil {
				v = nil
			}
		}()
		v = timer.Load()
	}()

	last, ok := v.(int64)
	if !ok || last == 0 {
		return "unknown"
	}

	age := time.Now().Unix() - last
	if age < 0 {
		age = 0
	}
	return strconv.FormatInt(age, 10) + "s"
}

// ControlAgent will check if the camera is still connected, if not it will restart the agent.
// In the other thread we are keeping track of the number of complete video access units received.
// Once we are not receiving any packets anymore, we will restart the agent.
func ControlAgent(communication *models.Communication) {
	log.Log.Debug("components.Kerberos.ControlAgent(): started")
	packageCounter := communication.PackageCounter
	packageSubCounter := communication.PackageCounterSub
	go func() {
		watchdog := newStreamRestartWatchdog()
		for {
			time.Sleep(streamWatchdogInterval)
			if !communication.CameraConnected.Load() {
				watchdog.ResetObservations()
				continue
			}

			packetsR := packageCounter.Load().(int64)
			packetsSubR := packageSubCounter.Load().(int64)
			log.Log.Info("components.Kerberos.ControlAgent(): Number of packets read from mainstream: " + strconv.FormatInt(packetsR, 10))
			subStreamConnected := communication.SubStreamConnected.Load()
			if subStreamConnected {
				log.Log.Info("components.Kerberos.ControlAgent(): Number of packets read from substream: " + strconv.FormatInt(packetsSubR, 10))
			}

			reason, restart := watchdog.Observe(time.Now(), packetsR, packetsSubR, subStreamConnected, communication.IsConfiguring.IsSet())
			if !restart {
				communication.SetWatchdogCooldown(watchdog.CooldownRemaining(time.Now()))
				continue
			}

			log.Log.Info(fmt.Sprintf(
				"components.Kerberos.ControlAgent(): Restarting machinery because of blocking %s. (mainPackets=%d, subPackets=%d, mainLastPacket=%s ago, subLastPacket=%s ago, nextBackoff=%s)",
				reason, packetsR, packetsSubR, packetAgeString(communication.LastPacketTimer), packetAgeString(communication.LastPacketTimerSub), watchdog.Backoff(),
			))
			select {
			case communication.HandleBootstrap <- "restart":
				cooldown := watchdog.MarkRestart(time.Now())
				communication.RecordWatchdogRestart(cooldown)
			case <-time.After(time.Second):
				log.Log.Info("components.Kerberos.ControlAgent(): Restarting machinery timed out")
			}
		}
	}()
	log.Log.Debug("components.Kerberos.ControlAgent(): finished")
}

const (
	runShutdownTimeout         = 10 * time.Second
	streamWatchdogInterval     = 5 * time.Second
	streamWatchdogStallChecks  = 3
	streamWatchdogBaseBackoff  = 15 * time.Second
	streamWatchdogMaxBackoff   = 2 * time.Minute
	streamWatchdogHealthyReset = time.Minute
)

type streamRestartWatchdog struct {
	previousMain    int64
	previousSub     int64
	mainStalls      int
	subStalls       int
	backoff         time.Duration
	nextRestart     time.Time
	healthySince    time.Time
	hasObservations bool
}

func newStreamRestartWatchdog() *streamRestartWatchdog {
	return &streamRestartWatchdog{backoff: streamWatchdogBaseBackoff}
}

func (w *streamRestartWatchdog) ResetObservations() {
	w.mainStalls = 0
	w.subStalls = 0
	w.healthySince = time.Time{}
	w.hasObservations = false
}

func (w *streamRestartWatchdog) Observe(now time.Time, mainPackets, subPackets int64, subConnected, configuring bool) (string, bool) {
	if !w.hasObservations {
		w.previousMain = mainPackets
		w.previousSub = subPackets
		w.hasObservations = true
		return "", false
	}

	mainHealthy := mainPackets != w.previousMain
	subHealthy := !subConnected || subPackets != w.previousSub
	w.previousMain = mainPackets
	w.previousSub = subPackets

	if mainHealthy && subHealthy {
		w.mainStalls = 0
		w.subStalls = 0
		if w.healthySince.IsZero() {
			w.healthySince = now
		} else if now.Sub(w.healthySince) >= streamWatchdogHealthyReset {
			w.backoff = streamWatchdogBaseBackoff
			w.nextRestart = time.Time{}
		}
	} else {
		w.healthySince = time.Time{}
	}

	if configuring || now.Before(w.nextRestart) {
		return "", false
	}
	if mainHealthy {
		w.mainStalls = 0
	} else {
		w.mainStalls++
	}
	if subHealthy {
		w.subStalls = 0
	} else {
		w.subStalls++
	}

	mainStalled := w.mainStalls >= streamWatchdogStallChecks
	subStalled := w.subStalls >= streamWatchdogStallChecks
	if !mainStalled && !subStalled {
		return "", false
	}
	if mainStalled && subStalled {
		return "main and sub streams", true
	}
	if mainStalled {
		return "main stream", true
	}
	return "sub stream", true
}

func (w *streamRestartWatchdog) MarkRestart(now time.Time) time.Duration {
	cooldown := w.backoff
	w.nextRestart = now.Add(w.backoff)
	w.mainStalls = 0
	w.subStalls = 0
	if w.backoff < streamWatchdogMaxBackoff {
		w.backoff *= 2
		if w.backoff > streamWatchdogMaxBackoff {
			w.backoff = streamWatchdogMaxBackoff
		}
	}
	return cooldown
}

func (w *streamRestartWatchdog) Backoff() time.Duration {
	return w.backoff
}

func (w *streamRestartWatchdog) CooldownRemaining(now time.Time) time.Duration {
	if !now.Before(w.nextRestart) {
		return 0
	}
	return w.nextRestart.Sub(now)
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
	cameraIsOnline := communication.CameraConnected.Load()

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
	activeWebRTCReaders := webrtc.GetActivePeerConnectionCount()
	pendingWebRTCHandshakes := communication.PendingLiveHDHandshakes()

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
		"webrtcReaders":      activeWebRTCReaders,
		"webrtcPending":      pendingWebRTCHandshakes,
		"recovery":           communication.RecoveryTelemetry(),
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
	if !communication.TrySendMotion(dataToPass) {
		c.JSON(503, gin.H{"recording": false, "error": "camera is restarting or recording queue is full"})
		return
	}
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
	// We'll try to get a fresh snapshot from the camera. Capturing a snapshot
	// reads a keyframe from the live stream, which blocks until one arrives.
	// When the camera is offline or the stream is stalled (no packets being
	// received) this would block the /config endpoint indefinitely, making the
	// agent appear unreachable even though its HTTP server is healthy. We
	// therefore bound the snapshot fetch with a short timeout and fall back to
	// the last cached snapshot, so /config always responds promptly.
	snapshot := make(chan string, 1)
	go func() {
		snapshot <- capture.Base64Image(captureDevice, communication, configuration)
	}()
	select {
	case base64Image := <-snapshot:
		if base64Image != "" {
			communication.Image = base64Image
		}
	case <-time.After(2 * time.Second):
		log.Log.Info("components.Kerberos.GetConfig(): snapshot timed out (stream stalled or camera offline), returning configuration with the last cached snapshot.")
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
