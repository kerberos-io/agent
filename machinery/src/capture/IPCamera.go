package capture

import (
	"strconv"
	"sync"
	"time"

	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/packets"
	"github.com/kerberos-io/joy4/av/pubsub"

	"github.com/kerberos-io/joy4/av"
	"github.com/kerberos-io/joy4/cgo/ffmpeg"
)

func DecodeImage(frame *ffmpeg.VideoFrame, pkt packets.Packet, decoder *ffmpeg.VideoDecoder, decoderMutex *sync.Mutex) (*ffmpeg.VideoFrame, error) {
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
