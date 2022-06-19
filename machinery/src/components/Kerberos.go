package components

import (
	"sync"
	"sync/atomic"

	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/joy4/av/pubsub"
	"github.com/kerberos-io/joy4/cgo/ffmpeg"
)

func Bootstrap(config *models.Config, log Logging) {
	log.Info("Start bootstrapping Agent")

	var packetsRead atomic.Value
	packetsRead.Store(int64(0))

	log.Info("Creating livestream and webrtc channel")
	livestreamChan := make(chan int64, 1)
	webrtcKeepAlive := make(chan string)
	webrtcPeers := make(chan string)
	webrtcChan := make(chan models.SDPPayload, 1)
	onvifActions := make(chan models.OnvifAction, 1)

	log.Info("Starting configuration MQTT")
	mqc := ConfigureMQTT(log, config, livestreamChan, webrtcChan, webrtcKeepAlive, webrtcPeers, onvifActions)

	// Ring buffer of 250 images. 25FPS = 10sec. or if set to 0, use 1 by default.
	preRecording := config.Capture.PreRecording * 25
	if preRecording < 3 {
		preRecording = 3
	}

	// A channel to trigger the motion
	motion := make(chan int64, 1)

	// A channel to stop the streaming loop
	stopHandleStream := make(chan int64, 1)

	// A channel to stop the uploading loop
	//stopUpload := make(chan int64, 1)

	// A channel to stop the sendalive loop
	//stopSendAlive := make(chan int64, 1)

	// open RTSP
	log.Info("Opening RTSP stream")
	rtspUrl := config.Capture.IPCamera.RTSP
	infile, streams, err := OpenRTSP(rtspUrl)
	var decoder *ffmpeg.VideoDecoder

	if err == nil {

		var decoderMutex sync.Mutex
		decoder = GetVideoDecoder(streams)

		var queue *pubsub.Queue
		queue = pubsub.NewQueue()
		queue.SetMaxGopCount(5) // GOP time frame is set to 5.
		queue.WriteHeader(streams)
		recordingCursor := queue.Oldest()
		//livestreamCursor := queue.Oldest()
		motionCursor := queue.Oldest()
		//webrtcCursor := queue.Oldest()

		name := config.Name
		go RecordStream(log, recordingCursor, motion, name, config, streams)
		go ProcessMotion(log, motionCursor, config, name, mqc, motion, decoder, &decoderMutex)
		go HandleStream(log, queue, stopHandleStream, &packetsRead, infile)
		//go cloud.StartUpload(log, config, name, stopUpload)
		//go cloud.SendStillAlive(config, stopSendAlive)
		//go cloud.SendLiveStream(config, livestreamCursor, mqc, decoder, &decoderMutex, log, livestreamChan)
		//go cloud.SendWebRTCStream(config, webrtcCursor, mqc, streams, log, webrtcChan, webrtcKeepAlive, webrtcPeers, decoder, &decoderMutex)
		//go onvif.HandleActions(log, config, onvifActions)

	} else {
		log.Error("Something went wrong while opening RTSP: " + err.Error())
	}

}
