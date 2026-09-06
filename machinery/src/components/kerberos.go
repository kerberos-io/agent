package components

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"

	"github.com/kerberos-io/agent/machinery/src/capture"
	"github.com/kerberos-io/agent/machinery/src/cloud"
	"github.com/kerberos-io/agent/machinery/src/computervision"
	configService "github.com/kerberos-io/agent/machinery/src/config"
	"github.com/kerberos-io/agent/machinery/src/lifecycle"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/onvif"
	"github.com/kerberos-io/agent/machinery/src/packets"
	routers "github.com/kerberos-io/agent/machinery/src/routers/mqtt"
	"github.com/kerberos-io/agent/machinery/src/utils"
	"github.com/kerberos-io/agent/machinery/src/webrtc"
	log "github.com/sirupsen/logrus"
	"github.com/tevino/abool"
)

var tracer = otel.Tracer("github.com/kerberos-io/agent/machinery/src/components")

func Bootstrap(ctx context.Context, configDirectory string, configuration *models.Configuration, communication *models.Communication, captureDevice *capture.Capture) {

	log.WithFields(log.Fields{
		"component": "agent",
		"event":     "bootstrap_started",
	}).Debug("Bootstrapping Agent")

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
		if ctx.Err() != nil {
			log.WithError(context.Cause(ctx)).WithFields(log.Fields{
				"component": "agent",
				"event":     "bootstrap_stopped",
			}).Info("Agent bootstrap stopped")
			return
		}

		// This will blocking until receiving a signal to be restarted, reconfigured, stopped, etc.
		status := RunAgent(ctx, configDirectory, configuration, communication, mqttClient, uptimeStart, cameraSettings, captureDevice)

		if status == runStatusParentCanceled {
			log.WithFields(log.Fields{
				"component": "agent",
				"event":     "bootstrap_stopped",
				"status":    status,
			}).Info("Agent bootstrap stopped")
			return
		}
		if status == "stop" {
			log.WithFields(log.Fields{
				"component":     "agent",
				"delay_seconds": 3,
				"event":         "process_shutdown_requested",
			}).Info("Agent process shutdown requested")
			time.Sleep(time.Second * 3)
			os.Exit(0)
		}
		if status == runStatusShutdownTimeout || status == runStatusOwnershipConflict {
			log.WithFields(log.Fields{
				"component": "agent",
				"event":     "process_terminating",
				"status":    status,
			}).Error("Terminating Agent after camera workers failed to stop")
			os.Exit(1)
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

	}
}

func RunAgent(parent context.Context, configDirectory string, configuration *models.Configuration, communication *models.Communication, mqttClient mqtt.Client, uptimeStart time.Time, cameraSettings *models.Camera, captureDevice *capture.Capture) (status string) {
	if parent == nil {
		parent = context.Background()
	}

	ctxRunAgent, span := tracer.Start(parent, "RunAgent")
	defer span.End()

	config := configuration.Config

	status = "not started"
	communication.CameraConnected.Store(false)
	communication.MainStreamConnected.Store(false)
	communication.SubStreamConnected.Store(false)
	mainRTSP := config.Capture.IPCamera.RTSP
	subRTSP := config.Capture.IPCamera.SubRTSP
	communication.SetStreamConfigured(models.MainStream, mainRTSP != "")
	communication.SetStreamConfigured(models.SubStream, subRTSP != "" && subRTSP != mainRTSP)

	run := models.NewAgentRun(parent, communication, config.Offline != "true")
	runFields := log.Fields{
		"component": "agent_run",
		"run_id":    run.ID(),
	}
	log.WithFields(runFields).WithFields(log.Fields{
		"event":             "run_starting",
		"offline":           config.Offline,
		"substream_enabled": config.Capture.IPCamera.SubRTSP != "",
	}).Info("Starting camera run")
	runStopped := false
	shutdownCause := error(nil)
	var rtspClient *capture.Golibrtsp
	var rtspSubClient *capture.Golibrtsp
	var rtspBackChannelClient *capture.Golibrtsp
	run.SetClientRelease(func() {
		captureDevice.ClearClients(rtspClient, rtspSubClient, rtspBackChannelClient)
	})
	defer func() {
		if runStopped {
			return
		}
		report := shutdownAgentRun(run, communication, shutdownCause)
		if !report.Complete {
			status = runStatusShutdownTimeout
		}
	}()

	// Currently only support H264 encoded cameras, this will change.
	// Establishing the camera connection without backchannel if no substream
	rtspUrl := mainRTSP
	rtspClient = captureDevice.SetMainClient(rtspUrl)
	run.SetMainClient(rtspClient)
	if rtspUrl != "" {
		err := rtspClient.Connect(run.Context(), ctxRunAgent)
		if err != nil {
			log.WithError(err).WithFields(runFields).WithFields(log.Fields{
				"event":  "rtsp_connect_failed",
				"stream": "main",
			}).Error("Failed to connect RTSP stream")
			shutdownCause = err
			if !waitForRunRetry(parent) {
				status = runStatusParentCanceled
				shutdownCause = context.Cause(parent)
			}
			return status
		}
	} else {
		log.WithFields(runFields).WithFields(log.Fields{
			"event":  "rtsp_configuration_missing",
			"stream": "main",
		}).Error("Main RTSP URL is not configured")
		shutdownCause = errors.New("no RTSP URL configured")
		if !waitForRunRetry(parent) {
			status = runStatusParentCanceled
			shutdownCause = context.Cause(parent)
		}
		return status
	}

	log.WithFields(runFields).WithFields(log.Fields{
		"event":  "rtsp_connected",
		"stream": "main",
	}).Info("RTSP stream connected")

	// Get the video streams from the RTSP server.
	videoStreams, err := rtspClient.GetVideoStreams()
	if err != nil || len(videoStreams) == 0 {
		entry := log.WithFields(runFields).WithFields(log.Fields{
			"event":        "rtsp_video_missing",
			"stream":       "main",
			"stream_count": len(videoStreams),
		})
		if err != nil {
			entry = entry.WithError(err)
		}
		entry.Error("RTSP stream has no supported video track")
		shutdownCause = errors.New("main RTSP stream has no supported video track")
		if err != nil {
			shutdownCause = err
		}
		if !waitForRunRetry(parent) {
			status = runStatusParentCanceled
			shutdownCause = context.Cause(parent)
		}
		return status
	}

	// Get the video stream from the RTSP server.
	videoStream := videoStreams[0]
	log.WithFields(runFields).WithFields(log.Fields{
		"codec":  videoStream.Name,
		"event":  "video_stream_detected",
		"fps":    videoStream.FPS,
		"height": videoStream.Height,
		"stream": "main",
		"width":  videoStream.Width,
	}).Info("Video stream detected")

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
		log.WithFields(runFields).WithFields(log.Fields{
			"event":                 "prerecording_adjusted",
			"pre_recording_seconds": config.Capture.PreRecording,
		}).Warn("Invalid prerecording duration; using one second")
	}

	// We might have a secondary rtsp url, so we might need to use that for livestreaming let us check first!
	subStreamEnabled := false
	subRtspUrl := config.Capture.IPCamera.SubRTSP
	var videoSubStreams []packets.Stream

	if subRtspUrl != "" && subRtspUrl != rtspUrl {
		// For the sub stream we will not enable backchannel.
		subStreamEnabled = true
		rtspSubClient = captureDevice.SetSubClient(subRtspUrl)

		run.SetSubClient(rtspSubClient)

		err := rtspSubClient.Connect(run.Context(), ctxRunAgent)
		if err != nil {
			log.WithError(err).WithFields(runFields).WithFields(log.Fields{
				"event":  "rtsp_connect_failed",
				"stream": "sub",
			}).Error("Failed to connect RTSP stream")
			shutdownCause = err
			if !waitForRunRetry(parent) {
				status = runStatusParentCanceled
				shutdownCause = context.Cause(parent)
			}
			return status
		}
		log.WithFields(runFields).WithFields(log.Fields{
			"event":  "rtsp_connected",
			"stream": "sub",
		}).Info("RTSP stream connected")

		// Get the video streams from the RTSP server.
		videoSubStreams, err = rtspSubClient.GetVideoStreams()
		if err != nil || len(videoSubStreams) == 0 {
			entry := log.WithFields(runFields).WithFields(log.Fields{
				"event":        "rtsp_video_missing",
				"stream":       "sub",
				"stream_count": len(videoSubStreams),
			})
			if err != nil {
				entry = entry.WithError(err)
			}
			entry.Error("RTSP stream has no supported video track")
			shutdownCause = errors.New("sub RTSP stream has no supported video track")
			if err != nil {
				shutdownCause = err
			}
			if !waitForRunRetry(parent) {
				status = runStatusParentCanceled
				shutdownCause = context.Cause(parent)
			}
			return status
		}

		// Get the video stream from the RTSP server.
		videoSubStream := videoSubStreams[0]
		log.WithFields(runFields).WithFields(log.Fields{
			"codec":  videoSubStream.Name,
			"event":  "video_stream_detected",
			"fps":    videoSubStream.FPS,
			"height": videoSubStream.Height,
			"stream": "sub",
			"width":  videoSubStream.Width,
		}).Info("Video stream detected")

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
	run.SetMainQueue(queue)

	// Set the maximum GOP count, this is used to determine the pre-recording time.
	const initialMaxGOPCount = 1
	queue.SetMaxGopCount(initialMaxGOPCount) // Adjusted after the GOP duration is observed.
	log.WithFields(runFields).WithFields(log.Fields{
		"event":         "packet_queue_configured",
		"max_gop_count": initialMaxGOPCount,
		"stream":        "main",
	}).Debug("Packet queue configured")
	queue.WriteHeader(videoStreams)
	if err := run.Activate(); err != nil {
		log.WithError(err).WithFields(runFields).WithField("event", "run_activation_failed").
			Error("Failed to activate camera run")
		if parent.Err() != nil {
			status = runStatusParentCanceled
			shutdownCause = context.Cause(parent)
		} else {
			status = runStatusOwnershipConflict
			shutdownCause = err
		}
		return status
	}

	var taskRegistrationErr error
	registerTask := func(name string, policy lifecycle.TaskPolicy, task lifecycle.TaskFunc) {
		if taskRegistrationErr != nil {
			return
		}
		taskRegistrationErr = run.Go(name, policy, task)
	}

	registerTask("rtsp-main-start", lifecycle.TaskPolicy{Required: true}, func(taskContext context.Context) error {
		return rtspClient.Start(taskContext, "main", queue, configuration, communication)
	})

	// Main stream is connected and ready to go.
	communication.MainStreamConnected.Store(true)

	// Try to create backchannel
	communication.HasBackChannel.Store(false)
	rtspBackChannelClient = captureDevice.SetBackChannelClient(rtspUrl)
	run.SetBackchannelClient(rtspBackChannelClient)
	err = rtspBackChannelClient.ConnectBackChannel(run.Context(), ctxRunAgent)
	if err == nil {
		log.WithFields(runFields).WithFields(log.Fields{
			"event":  "rtsp_connected",
			"stream": "backchannel",
		}).Info("RTSP stream connected")
	} else {
		log.WithError(err).WithFields(runFields).WithFields(log.Fields{
			"event":  "rtsp_backchannel_unavailable",
			"stream": "backchannel",
		}).Debug("Optional RTSP backchannel is unavailable")
	}

	if subStreamEnabled && rtspSubClient != nil {
		subQueue = packets.NewQueue()
		run.SetSubQueue(subQueue)
		subQueue.SetMaxGopCount(1) // GOP time frame is set to 1 for motion detection and livestreaming.
		subQueue.WriteHeader(videoSubStreams)
		registerTask("rtsp-sub-start", lifecycle.TaskPolicy{Required: true}, func(taskContext context.Context) error {
			return rtspSubClient.Start(taskContext, "sub", subQueue, configuration, communication)
		})

		// Sub stream is connected and ready to go.
		communication.SubStreamConnected.Store(true)
	}

	// Handle livestream SD (low resolution over MQTT)
	if subStreamEnabled {
		livestreamCursor := subQueue.Latest()
		registerTask("live-sd", lifecycle.TaskPolicy{}, func(context.Context) error {
			cloud.HandleLiveStreamSD(livestreamCursor, configuration, communication, mqttClient, rtspSubClient)
			return nil
		})
	} else {
		livestreamCursor := queue.Latest()
		registerTask("live-sd", lifecycle.TaskPolicy{}, func(context.Context) error {
			cloud.HandleLiveStreamSD(livestreamCursor, configuration, communication, mqttClient, rtspClient)
			return nil
		})
	}

	// Handle livestream HLS (adaptive segments over HTTP via hub-api -> vault).
	// The producer can serve either the main (high-resolution) or sub
	// (low-resolution) stream and switches between them on demand based on the
	// quality the viewer requests; "auto" prefers the sub stream when available.
	// Like SD it is viewer-keepalive gated and produces no traffic while nobody is
	// watching.
	registerTask("live-hls", lifecycle.TaskPolicy{}, func(context.Context) error {
		cloud.HandleLiveStreamHLS(configuration, communication, mqttClient, subStreamEnabled, queue, subQueue)
		return nil
	})

	// MoQ is available only in the dedicated CGO/glibc build. The standard
	// static Alpine build resolves this hook to a no-op.
	registerTask("live-moq", lifecycle.TaskPolicy{}, func(taskContext context.Context) error {
		cloud.StartLiveStreamMoQ(taskContext, configuration, communication, subStreamEnabled, queue, subQueue)
		return nil
	})

	// Handle livestream HD (high resolution over WEBRTC). Both the main and sub
	// stream are exposed as separate broadcasters so a viewer can request the
	// high (main) or low (sub) resolution per peer connection; "auto" prefers the
	// sub stream when available.
	liveHDHandshakes := run.LiveHDHandshakes()
	motionEvents := run.MotionEvents()
	onvifActions := run.ONVIFActions()
	registerTask("live-hd", lifecycle.TaskPolicy{}, func(context.Context) error {
		cloud.HandleLiveStreamHD(
			configuration,
			communication,
			mqttClient,
			rtspClient,
			rtspSubClient,
			subStreamEnabled,
			liveHDHandshakes,
			queue,
			subQueue,
		)
		return nil
	})

	// Handle recording, will write an mp4 to disk.
	recordingPolicy := lifecycle.TaskPolicy{}
	if config.Capture.Recording != "false" {
		recordingPolicy = lifecycle.TaskPolicy{Required: true, LongRunning: true}
	}
	registerTask("recording", recordingPolicy, func(context.Context) error {
		capture.HandleRecordStream(queue, configDirectory, configuration, communication, rtspClient, mqttClient, motionEvents)
		return nil
	})

	// Handle processing of motion
	if subStreamEnabled {
		motionCursor := subQueue.Latest()
		registerTask("motion", lifecycle.TaskPolicy{}, func(context.Context) error {
			computervision.ProcessMotion(motionCursor, configuration, communication, mqttClient, rtspSubClient)
			return nil
		})
	} else {
		motionCursor := queue.Latest()
		registerTask("motion", lifecycle.TaskPolicy{}, func(context.Context) error {
			computervision.ProcessMotion(motionCursor, configuration, communication, mqttClient, rtspClient)
			return nil
		})
	}

	// Handle realtime processing if enabled.
	if subStreamEnabled {
		realtimeProcessingCursor := subQueue.Latest()
		registerTask("realtime-processing", lifecycle.TaskPolicy{}, func(context.Context) error {
			cloud.HandleRealtimeProcessing(realtimeProcessingCursor, configuration, communication, mqttClient, rtspClient)
			return nil
		})
	} else {
		realtimeProcessingCursor := queue.Latest()
		registerTask("realtime-processing", lifecycle.TaskPolicy{}, func(context.Context) error {
			cloud.HandleRealtimeProcessing(realtimeProcessingCursor, configuration, communication, mqttClient, rtspClient)
			return nil
		})
	}

	// Handle Upload to cloud provider (Kerberos Hub, Kerberos Vault and others)
	registerTask("upload", lifecycle.TaskPolicy{}, func(context.Context) error {
		cloud.HandleUpload(configDirectory, configuration, communication)
		return nil
	})

	// Handle ONVIF actions
	registerTask("onvif-actions", lifecycle.TaskPolicy{}, func(context.Context) error {
		onvif.HandleONVIFActions(configuration, communication, onvifActions)
		return nil
	})

	// Handle ONVIF event stream — opt-in via Capture.ONVIFMotion="true".
	// Stops when the agent's shared context is cancelled. The function
	// is a no-op if ONVIFMotion is not enabled.
	registerTask("onvif-events", lifecycle.TaskPolicy{}, func(taskContext context.Context) error {
		onvif.HandleONVIFEventStream(taskContext, configuration, communication)
		return nil
	})

	if rtspBackChannelClient.HasBackChannel {
		communication.HasBackChannel.Store(true)
		registerTask("backchannel", lifecycle.TaskPolicy{}, func(taskContext context.Context) error {
			WriteAudioToBackchannel(taskContext, communication, rtspBackChannelClient)
			return nil
		})
	}

	if taskRegistrationErr != nil {
		log.WithError(taskRegistrationErr).WithFields(runFields).
			WithField("event", "task_registration_failed").
			Error("Failed to register camera task")
		if parent.Err() != nil {
			status = runStatusParentCanceled
			shutdownCause = context.Cause(parent)
		} else {
			status = "restart"
			shutdownCause = taskRegistrationErr
		}
	} else {
		run.Seal()

		// If we reach this point, we have a working RTSP connection.
		communication.CameraConnected.Store(true)

		select {
		case status = <-communication.HandleBootstrap:
			shutdownCause = fmt.Errorf("camera run requested %s", status)
		case failure := <-run.Failures():
			entry := log.WithError(failure.Cause).WithFields(runFields).WithFields(log.Fields{
				"event": "task_failed",
				"panic": failure.Panic != "",
				"task":  failure.Task,
			})
			entry.Error("Supervised Agent task failed")
			if len(failure.Stack) > 0 {
				entry.WithField("stack", string(failure.Stack)).
					Debug("Supervised Agent task panic stack")
			}
			status = "restart"
			shutdownCause = failure.Cause
		case <-parent.Done():
			status = runStatusParentCanceled
			shutdownCause = context.Cause(parent)
		}
	}

	// If we reach this point, we are stopping the stream.
	communication.CameraConnected.Store(false)
	communication.MainStreamConnected.Store(false)
	communication.SubStreamConnected.Store(false)

	shutdownReport := shutdownAgentRun(run, communication, shutdownCause)
	runStopped = true
	if shutdownReport.Complete {
		log.WithFields(runFields).WithFields(log.Fields{
			"event":      "workers_stopped",
			"status":     status,
			"task_count": len(shutdownReport.Tasks),
		}).Info("All Agent run workers stopped")
		for _, task := range shutdownReport.Tasks {
			if task.Status == lifecycle.TaskPanicked {
				entry := log.WithFields(runFields).WithFields(log.Fields{
					"event": "task_panicked",
					"panic": task.Panic,
					"task":  task.Name,
				})
				entry.Error("Agent task panicked during shutdown")
				if len(task.Stack) > 0 {
					entry.WithField("stack", string(task.Stack)).
						Debug("Agent task panic stack")
				}

				if status != "stop" {
					status = "restart"
				}
			}
		}
	} else {
		status = runStatusShutdownTimeout
	}

	if status == runStatusShutdownTimeout || status == runStatusOwnershipConflict || status == runStatusParentCanceled {
		return status
	}

	// Factory reads retry transient database failures, so release runtime
	// resources before reopening configuration.
	configService.OpenConfig(configDirectory, configuration)
	configService.OverrideWithEnvironmentVariables(configuration)

	return status
}

func shutdownAgentRun(run *models.AgentRun, communication *models.Communication, cause error) models.AgentRunShutdownReport {
	waitContext, cancelWait := context.WithTimeout(context.Background(), runShutdownTimeout)
	defer cancelWait()

	report := run.Shutdown(waitContext, cause)
	for _, resourceErr := range report.ResourceErrors {
		log.WithError(resourceErr.Err).WithFields(log.Fields{
			"component": "agent_run",
			"event":     "resource_close_failed",
			"resource":  resourceErr.Resource,
			"run_id":    run.ID(),
		}).Error("Agent run resource failed to close")
	}
	if report.Complete {
		return report
	}

	communication.RecordRunWorkerShutdownTimeout()
	log.WithError(cause).WithFields(log.Fields{
		"component":          "agent_run",
		"event":              "worker_shutdown_timeout",
		"run_id":             run.ID(),
		"running_task_count": len(report.Running),
		"timeout_ms":         runShutdownTimeout.Milliseconds(),
	}).Error("Timed out waiting for Agent run workers")
	for _, task := range report.Running {
		log.WithFields(log.Fields{
			"component":   "agent_run",
			"duration_ms": time.Since(task.StartedAt).Milliseconds(),
			"event":       "task_stuck",
			"run_id":      run.ID(),
			"task":        task.Name,
		}).Error("Agent task is still running after shutdown timeout")
	}
	return report
}

func waitForRunRetry(ctx context.Context) bool {
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func packetAgeSeconds(timer *atomic.Value) (int64, bool) {
	if timer == nil {
		return 0, false
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
		return 0, false
	}

	age := time.Now().Unix() - last
	if age < 0 {
		age = 0
	}
	return age, true
}

// ControlAgent will check if the camera is still connected, if not it will restart the agent.
// In the other thread we are keeping track of the number of complete video access units received.
// Once we are not receiving any packets anymore, we will restart the agent.
func ControlAgent(communication *models.Communication) {
	log.WithFields(log.Fields{
		"component":   "watchdog",
		"event":       "watchdog_started",
		"interval_ms": streamWatchdogInterval.Milliseconds(),
	}).Debug("Stream watchdog started")
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
			subStreamConnected := communication.SubStreamConnected.Load()
			log.WithFields(log.Fields{
				"component":           "watchdog",
				"event":               "packet_counters",
				"main_packets":        packetsR,
				"sub_packets":         packetsSubR,
				"substream_connected": subStreamConnected,
			}).Debug("Stream packet counters sampled")

			reason, restart := watchdog.Observe(time.Now(), packetsR, packetsSubR, subStreamConnected, communication.IsConfiguring.IsSet())
			if !restart {
				communication.SetWatchdogCooldown(watchdog.CooldownRemaining(time.Now()))
				continue
			}

			mainPacketAgeSeconds, mainPacketAgeKnown := packetAgeSeconds(communication.LastPacketTimer)
			subPacketAgeSeconds, subPacketAgeKnown := packetAgeSeconds(communication.LastPacketTimerSub)
			log.WithFields(log.Fields{
				"component":                    "watchdog",
				"event":                        "stream_restart_requested",
				"main_last_packet_age_known":   mainPacketAgeKnown,
				"main_last_packet_age_seconds": mainPacketAgeSeconds,
				"main_packets":                 packetsR,
				"next_backoff_ms":              watchdog.Backoff().Milliseconds(),
				"reason":                       reason,
				"sub_last_packet_age_known":    subPacketAgeKnown,
				"sub_last_packet_age_seconds":  subPacketAgeSeconds,
				"sub_packets":                  packetsSubR,
			}).Warn("Stream watchdog requested an Agent restart")

			select {
			case communication.HandleBootstrap <- "restart":
				cooldown := watchdog.MarkRestart(time.Now())
				communication.RecordWatchdogRestart(cooldown)
			case <-time.After(time.Second):
				log.WithFields(log.Fields{
					"component":  "watchdog",
					"event":      "restart_signal_timeout",
					"timeout_ms": time.Second.Milliseconds(),
				}).Error("Timed out delivering watchdog restart signal")
			}
		}
	}()
	log.WithFields(log.Fields{
		"component": "watchdog",
		"event":     "watchdog_worker_started",
	}).Debug("Stream watchdog worker started")
}

const (
	runStatusOwnershipConflict = "run ownership conflict"
	runStatusParentCanceled    = "parent canceled"
	runStatusShutdownTimeout   = "shutdown timed out"
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
	log.Info("components.Kerberos.StopAgent(): sending signal to stop agent, this will os.Exit(0).")
	select {
	case communication.HandleBootstrap <- "stop":
		log.Info("components.Kerberos.StopAgent(): Stopping machinery.")
	case <-time.After(1 * time.Second):
		log.Info("components.Kerberos.StopAgent(): Stopping machinery timed out")
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
	log.Info("components.Kerberos.RestartAgent(): sending signal to restart agent.")
	select {
	case communication.HandleBootstrap <- "restart":
		log.Info("components.Kerberos.RestartAgent(): Restarting machinery.")
	case <-time.After(1 * time.Second):
		log.Info("components.Kerberos.RestartAgent(): Restarting machinery timed out")
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
	log.Info("components.Kerberos.MakeRecording(): sending signal to start recording.")
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
		log.Info("components.Kerberos.GetConfig(): snapshot timed out (stream stalled or camera offline), returning configuration with the last cached snapshot.")
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
