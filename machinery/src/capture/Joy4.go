package capture

import (
	"context"
	"image"
	"strconv"
	"sync"
	"time"

	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/packets"
	"github.com/kerberos-io/joy4/av"
	"github.com/kerberos-io/joy4/av/avutil"
	"github.com/kerberos-io/joy4/cgo/ffmpeg"
	"github.com/kerberos-io/joy4/codec/h264parser"
	"github.com/kerberos-io/joy4/format"
)

// Implements the RTSPClient interface.
type Joy4 struct {
	RTSPClient
	Url             string
	WithBackChannel bool

	Demuxer av.DemuxCloser
	Streams []packets.Stream

	DecoderMutex *sync.Mutex
	Decoder      *ffmpeg.VideoDecoder
	Frame        *ffmpeg.VideoFrame
}

// Connect to the RTSP server.
func (j *Joy4) Connect(ctx context.Context) (err error) {

	// Register all formats and codecs.
	format.RegisterAll()

	// Try with backchannel first (if variable is set to true)
	// If set to true, it will try to open the stream with a backchannel
	// If fails we will try again (see below).
	infile, err := avutil.Open(ctx, j.Url, j.WithBackChannel)
	if err == nil {
		s, err := infile.Streams()
		if err == nil && len(s) > 0 {
			j.Decoder = &ffmpeg.VideoDecoder{}
			var streams []packets.Stream
			for _, str := range s {
				stream := packets.Stream{
					Name:    str.Type().String(),
					IsVideo: str.Type().IsVideo(),
					IsAudio: str.Type().IsAudio(),
				}
				if stream.IsVideo {
					num, denum := str.(av.VideoCodecData).Framerate()
					stream.Num = num
					stream.Denum = denum
					width := str.(av.VideoCodecData).Width()
					stream.Width = width
					height := str.(av.VideoCodecData).Height()
					stream.Height = height

					if stream.Name == "H264" {
						stream.PPS = str.(h264parser.CodecData).PPS()
						stream.SPS = str.(h264parser.CodecData).SPS()
					}

					// Specific to Joy4, we need to create a decoder.
					codec := str.(av.VideoCodecData)
					ffmpeg.NewVideoDecoder(j.Decoder, codec)
					err := ffmpeg.NewVideoDecoder(j.Decoder, codec)
					if err != nil {
						log.Log.Error("RTSPClient(JOY4).Connect(): " + err.Error())
					}
				}
				streams = append(streams, stream)
			}
			j.Demuxer = infile
			j.Streams = streams
		} else {
			// Try again without backchannel
			log.Log.Info("OpenRTSP: trying without backchannel")
			j.WithBackChannel = false
			infile, err := avutil.Open(ctx, j.Url, j.WithBackChannel)
			if err == nil {
				var streams []packets.Stream
				for _, str := range s {
					stream := packets.Stream{
						Name:    str.Type().String(),
						IsVideo: str.Type().IsVideo(),
						IsAudio: str.Type().IsAudio(),
					}
					if stream.IsVideo {
						num, denum := str.(av.VideoCodecData).Framerate()
						stream.Num = num
						stream.Denum = denum
						width := str.(av.VideoCodecData).Width()
						stream.Width = width
						height := str.(av.VideoCodecData).Height()
						stream.Height = height

						if stream.Name == "H264" {
							stream.PPS = str.(h264parser.CodecData).PPS()
							stream.SPS = str.(h264parser.CodecData).SPS()
						}

						// Specific to Joy4, we need to create a decoder.
						codec := str.(av.VideoCodecData)
						ffmpeg.NewVideoDecoder(j.Decoder, codec)
						err := ffmpeg.NewVideoDecoder(j.Decoder, codec)
						if err != nil {
							log.Log.Error("RTSPClient(JOY4).Connect(): " + err.Error())
						}
					}
					streams = append(streams, stream)
				}
				j.Demuxer = infile
				j.Streams = streams
			}
		}
	}

	// Create a single frame used for decoding.
	j.Frame = ffmpeg.AllocVideoFrame()

	// Iniatlise the mutex.
	j.DecoderMutex = &sync.Mutex{}

	return
}

// Start the RTSP client, and start reading packets.
func (j *Joy4) Start(ctx context.Context, queue *packets.Queue, communication *models.Communication) (err error) {
	log.Log.Debug("RTSPClient(JOY4).Start(): started")
	start := false
loop:
	for {
		// This will check if we need to stop the thread,
		// because of a reconfiguration.
		select {
		case <-communication.HandleStream:
			break loop
		default:
		}

		var avpkt av.Packet
		if avpkt, err = j.Demuxer.ReadPacket(); err != nil { // sometimes this throws an end of file..
			log.Log.Error("RTSPClient(JOY4).Start(): " + err.Error())
			time.Sleep(1 * time.Second)
		}

		// Could be that a decode is throwing errors.
		if len(avpkt.Data) > 0 {

			avpkt.Data = avpkt.Data[4:]
			if avpkt.IsKeyFrame {
				start = true
				// Add SPS and PPS to the packet.
				stream := j.Streams[avpkt.Idx]
				annexbNALUStartCode := func() []byte { return []byte{0x00, 0x00, 0x00, 0x01} }
				avpkt.Data = append(annexbNALUStartCode(), avpkt.Data...)
				avpkt.Data = append(stream.PPS, avpkt.Data...)
				avpkt.Data = append(annexbNALUStartCode(), avpkt.Data...)
				avpkt.Data = append(stream.SPS, avpkt.Data...)
				avpkt.Data = append(annexbNALUStartCode(), avpkt.Data...)
			}

			if start {
				// Conver to packet.
				pkt := packets.Packet{
					IsKeyFrame:      avpkt.IsKeyFrame,
					Idx:             int8(avpkt.Idx),
					CompositionTime: avpkt.CompositionTime,
					Time:            avpkt.Time,
					Data:            avpkt.Data,
				}

				queue.WritePacket(pkt)

				if pkt.IsKeyFrame {
					// Increment packets, so we know the device
					// is not blocking.
					r := communication.PackageCounter.Load().(int64)
					log.Log.Info("RTSPClient(JOY4).Start(): packet size " + strconv.Itoa(len(pkt.Data)))
					communication.PackageCounter.Store((r + 1) % 1000)
					communication.LastPacketTimer.Store(time.Now().Unix())
				}
			}

			// This will check if we need to stop the thread,
			// because of a reconfiguration.
			select {
			case <-communication.HandleStream:
				break loop
			default:
			}
		}
	}

	queue.Close()
	log.Log.Debug("RTSPClient(JOY4).Start(): done")

	return
}

// Decode a packet to an image.
func (j *Joy4) DecodePacket(pkt packets.Packet) (image.YCbCr, error) {
	j.DecoderMutex.Lock()
	_, err := j.Decoder.Decode(j.Frame, pkt.Data)
	j.DecoderMutex.Unlock()
	return j.Frame.Image, err
}

// Get a list of streams from the RTSP server.
func (j *Joy4) GetStreams() ([]packets.Stream, error) {
	var streams []packets.Stream
	for _, stream := range j.Streams {
		streams = append(streams, stream)
	}
	return streams, nil
}

// Get a list of video streams from the RTSP server.
func (j *Joy4) GetVideoStreams() ([]packets.Stream, error) {
	var videoStreams []packets.Stream
	for _, stream := range j.Streams {
		if stream.IsVideo {
			videoStreams = append(videoStreams, stream)
		}
	}
	return videoStreams, nil
}

// Get a list of audio streams from the RTSP server.
func (j *Joy4) GetAudioStreams() ([]packets.Stream, error) {
	var audioStreams []packets.Stream
	for _, stream := range j.Streams {
		if stream.IsAudio {
			audioStreams = append(audioStreams, stream)
		}
	}
	return audioStreams, nil
}

// Close the connection to the RTSP server.
func (j *Joy4) Close() error {
	// Cleanup the frame.
	j.Frame.Free()
	// Close the decoder.
	j.Decoder.Close()
	// Close the demuxer.
	return j.Demuxer.Close()
}
