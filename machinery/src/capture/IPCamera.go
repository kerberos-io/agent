package capture

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/joy4/av/pubsub"

	"github.com/kerberos-io/joy4/av"
	"github.com/kerberos-io/joy4/av/avutil"
	"github.com/kerberos-io/joy4/cgo/ffmpeg"
	"github.com/kerberos-io/joy4/format"
)

func OpenRTSP(ctx context.Context, url string, withBackChannel bool) (av.DemuxCloser, []av.CodecData, error) {
	format.RegisterAll()

	// Try with backchannel first (if variable is set to true)
	// If set to true, it will try to open the stream with a backchannel
	// If fails we will try again (see below).
	infile, err := avutil.Open(ctx, url, withBackChannel)
	if err == nil {
		streams, errstreams := infile.Streams()
		if len(streams) > 0 {
			return infile, streams, errstreams
		} else {
			// Try again without backchannel
			log.Log.Info("OpenRTSP: trying without backchannel")
			withBackChannel = false
			infile, err := avutil.Open(ctx, url, withBackChannel)
			if err == nil {
				streams, errstreams := infile.Streams()
				return infile, streams, errstreams
			}
		}
	}
	return nil, []av.CodecData{}, err
}

func GetVideoStream(streams []av.CodecData) (av.CodecData, error) {
	var videoStream av.CodecData
	for _, stream := range streams {
		if stream.Type().IsAudio() {
			//astream := stream.(av.AudioCodecData)
		} else if stream.Type().IsVideo() {
			videoStream = stream
		}
	}
	return videoStream, nil
}

func GetVideoDecoder(decoder *ffmpeg.VideoDecoder, streams []av.CodecData) {
	// Load video codec
	var vstream av.VideoCodecData
	for _, stream := range streams {
		if stream.Type().IsAudio() {
			//astream := stream.(av.AudioCodecData)
		} else if stream.Type().IsVideo() {
			vstream = stream.(av.VideoCodecData)
		}
	}
	err := ffmpeg.NewVideoDecoder(decoder, vstream)
	if err != nil {
		log.Log.Error("GetVideoDecoder: " + err.Error())
	}
}

func DecodeImage(frame *ffmpeg.VideoFrame, pkt av.Packet, decoder *ffmpeg.VideoDecoder, decoderMutex *sync.Mutex) (*ffmpeg.VideoFrame, error) {
	decoderMutex.Lock()
	img, err := decoder.Decode(frame, pkt.Data)
	decoderMutex.Unlock()
	return img, err
}

func HandleStream(infile av.DemuxCloser, queue *pubsub.Queue, communication *models.Communication) { //, wg *sync.WaitGroup) {

	log.Log.Debug("HandleStream: started")
	var err error
loop:
	for {
		// This will check if we need to stop the thread,
		// because of a reconfiguration.
		select {
		case <-communication.HandleStream:
			break loop
		default:
		}

		var pkt av.Packet
		if pkt, err = infile.ReadPacket(); err != nil { // sometimes this throws an end of file..
			log.Log.Error("HandleStream: " + err.Error())
			time.Sleep(1 * time.Second)
		}

		// Could be that a decode is throwing errors.
		if len(pkt.Data) > 0 {

			queue.WritePacket(pkt)

			// This will check if we need to stop the thread,
			// because of a reconfiguration.
			select {
			case <-communication.HandleStream:
				break loop
			default:
			}

			if pkt.IsKeyFrame {

				// Increment packets, so we know the device
				// is not blocking.
				r := communication.PackageCounter.Load().(int64)
				log.Log.Info("HandleStream: packet size " + strconv.Itoa(len(pkt.Data)))
				communication.PackageCounter.Store((r + 1) % 1000)
				communication.LastPacketTimer.Store(time.Now().Unix())
			}
		}
	}

	queue.Close()
	log.Log.Debug("HandleStream: finished")
}

func HandleSubStream(infile av.DemuxCloser, queue *pubsub.Queue, communication *models.Communication) { //, wg *sync.WaitGroup) {

	log.Log.Debug("HandleSubStream: started")
	var err error
loop:
	for {
		// This will check if we need to stop the thread,
		// because of a reconfiguration.
		select {
		case <-communication.HandleSubStream:
			break loop
		default:
		}

		var pkt av.Packet
		if pkt, err = infile.ReadPacket(); err != nil { // sometimes this throws an end of file..
			log.Log.Error("HandleSubStream: " + err.Error())
			time.Sleep(1 * time.Second)
		}

		// Could be that a decode is throwing errors.
		if len(pkt.Data) > 0 {

			queue.WritePacket(pkt)

			// This will check if we need to stop the thread,
			// because of a reconfiguration.
			select {
			case <-communication.HandleSubStream:
				break loop
			default:
			}
		}
	}

	queue.Close()
	log.Log.Debug("HandleSubStream: finished")
}
