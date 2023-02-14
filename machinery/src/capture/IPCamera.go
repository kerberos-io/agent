package capture

import (
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

func OpenRTSP(url string) (av.DemuxCloser, []av.CodecData, error) {
	format.RegisterAll()
	infile, err := avutil.Open(url)
	if err == nil {
		streams, errstreams := infile.Streams()
		return infile, streams, errstreams
	}
	return nil, []av.CodecData{}, err
}

func GetVideoDecoder(streams []av.CodecData) *ffmpeg.VideoDecoder {
	// Load video codec
	var vstream av.VideoCodecData
	for _, stream := range streams {
		if stream.Type().IsAudio() {
			//astream := stream.(av.AudioCodecData)
		} else if stream.Type().IsVideo() {
			vstream = stream.(av.VideoCodecData)
		}
	}
	dec, _ := ffmpeg.NewVideoDecoder(vstream)
	return dec
}

func DecodeImage(frame *ffmpeg.VideoFrame, pkt av.Packet, decoder *ffmpeg.VideoDecoder, decoderMutex *sync.Mutex) (*ffmpeg.VideoFrame, error) {
	decoderMutex.Lock()
	img, err := decoder.Decode(frame, pkt.Data)
	decoderMutex.Unlock()
	return img, err
}

func GetStreamInsights(infile av.DemuxCloser, streams []av.CodecData) (int, int, int, int) {
	var width, height, fps, gopsize int
	for _, stream := range streams {
		if stream.Type().IsAudio() {
			//astream := stream.(av.AudioCodecData)
		} else if stream.Type().IsVideo() {
			vstream := stream.(av.VideoCodecData)
			width = vstream.Width()
			height = vstream.Height()
		}
	}

loop:
	for timeout := time.After(1 * time.Second); ; {
		var err error
		if _, err = infile.ReadPacket(); err != nil { // sometimes this throws an end of file..
			log.Log.Error("HandleStream: " + err.Error())
		}
		fps++
		select {
		case <-timeout:
			break loop
		default:
		}
	}

	gopCounter := 0
	start := false
	for {
		var pkt av.Packet
		var err error
		if pkt, err = infile.ReadPacket(); err != nil { // sometimes this throws an end of file..
			log.Log.Error("HandleStream: " + err.Error())
		}
		// Could be that a decode is throwing errors.
		if len(pkt.Data) > 0 {
			if start {
				gopCounter = gopCounter + 1
			}

			if pkt.IsKeyFrame {
				if start == false {
					start = true
				} else {
					gopsize = gopCounter
					break
				}
			}
		}
	}

	return width, height, fps, gopsize
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
