package capture

import (
	"strconv"
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
			log.Log.Info(strconv.Itoa(len(pkt.Data)))
			log.Log.Error(err.Error())
			if err.Error() == "EOF" {
				time.Sleep(30 * time.Second)
			}
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

			/*select {
			case packetsBuffer <- pkt:
			default:
			}

			select {
			case webrtcPacketsRealtimeStream <- pkt:
			default:
			}*/

			if pkt.IsKeyFrame {

				// Increment packets, so we know the device
				// is not blocking.
				r := communication.PackageCounter.Load().(int64)
				log.Log.Info("HandleStream: packet size " + strconv.Itoa(len(pkt.Data)))
				communication.PackageCounter.Store((r + 1) % 1000)

				/*select {
				case packetsRealtime <- pkt:
				default:
				}
				select {
				case packetsRealtimeStream <- pkt:
				default:
				}*/
			}
		}
	}
	//wg.Done()

	queue.Close()
	log.Log.Debug("HandleStream: finished")
}
