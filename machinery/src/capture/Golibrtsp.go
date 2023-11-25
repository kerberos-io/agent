package capture

import (
	"context"
	"image"
	"strconv"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtph264"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/packets"
	"github.com/pion/rtp"
)

// Implements the RTSPClient interface.
type Golibrtsp struct {
	RTSPClient
	Url             string
	WithBackChannel bool

	Client  gortsplib.Client
	Media   *description.Media
	Forma   *format.H264
	Streams []packets.Stream

	DecoderMutex *sync.Mutex
	Decoder      *rtph264.Decoder
	//FrameDecoder *h264Decoder
}

// Connect to the RTSP server.
func (g *Golibrtsp) Connect(ctx context.Context) (err error) {

	g.Client = gortsplib.Client{}

	// parse URL
	u, err := base.ParseURL(g.Url)
	if err != nil {
		panic(err)
	}

	// connect to the server
	err = g.Client.Start(u.Scheme, u.Host)
	if err != nil {
		panic(err)
	}

	// find published medias
	desc, _, err := g.Client.Describe(u)
	if err != nil {
		panic(err)
	}

	// find the H264 media and format
	var forma *format.H264
	medi := desc.FindFormat(&forma)
	if medi == nil {
		panic("media not found")
	}
	g.Media = medi
	g.Forma = forma

	g.Streams = append(g.Streams, packets.Stream{
		Name:    forma.Codec(),
		IsVideo: true,
		IsAudio: false,
		SPS:     forma.SPS,
		PPS:     forma.PPS,
	})

	// setup RTP/H264 -> H264 decoder
	rtpDec, err := forma.CreateDecoder()
	if err != nil {
		panic(err)
	}
	g.Decoder = rtpDec

	// setup H264 -> raw frames decoder
	/*frameDec, err := newH264Decoder()
	if err != nil {
		panic(err)
	}
	g.FrameDecoder = frameDec

	// if SPS and PPS are present into the SDP, send them to the decoder
	if forma.SPS != nil {
		frameDec.decode(forma.SPS)
	}
	if forma.PPS != nil {
		frameDec.decode(forma.PPS)
	}*/

	// setup a single media
	_, err = g.Client.Setup(desc.BaseURL, medi, 0, 0)
	if err != nil {
		panic(err)
	}

	return
}

// Start the RTSP client, and start reading packets.
func (g *Golibrtsp) Start(ctx context.Context, queue *packets.Queue, communication *models.Communication) (err error) {
	log.Log.Debug("RTSPClient(Golibrtsp).Start(): started")

	// called when a RTP packet arrives
	g.Client.OnPacketRTP(g.Media, g.Forma, func(rtppkt *rtp.Packet) {

		// This will check if we need to stop the thread,
		// because of a reconfiguration.
		select {
		case <-communication.HandleStream:
			return
		default:
		}

		//og.Log.Info("RTSPClient(Golibrtsp).Start(): " + "read packet from stream: " + strconv.Itoa(len(pkt.Payload)) + " bytes")
		if len(rtppkt.Payload) > 0 {

			// extract access units from RTP packets
			au, err := g.Decoder.Decode(rtppkt)
			if err != nil {
				if err != rtph264.ErrNonStartingPacketAndNoPrevious && err != rtph264.ErrMorePacketsNeeded {
					log.Log.Error("RTSPClient(Golibrtsp).Start(): " + err.Error())
				}
				return
			}

			isKeyFrame := h264.IDRPresent(au)

			// Conver to packet.
			pkt := packets.Packet{
				IsKeyFrame:      isKeyFrame,
				Idx:             0,
				CompositionTime: time.Duration(rtppkt.Timestamp),
				Time:            time.Duration(rtppkt.Timestamp),
				Data:            rtppkt.Payload,
				Header: packets.Header{
					Version:        rtppkt.Version,
					Padding:        rtppkt.Padding,
					Extension:      rtppkt.Extension,
					Marker:         rtppkt.Marker,
					PayloadType:    rtppkt.PayloadType,
					SequenceNumber: rtppkt.SequenceNumber,
					Timestamp:      rtppkt.Timestamp,
					SSRC:           rtppkt.SSRC,
					CSRC:           rtppkt.CSRC,
				},
				PaddingSize: rtppkt.PaddingSize,
			}

			queue.WritePacket(pkt)

			/*for _, nalu := range au {
				// convert NALUs into RGBA frames
				img, err := g.FrameDecoder.decode(nalu)
				if err != nil {
					panic(err)
				}
				fmt.Println(img)
			}*/

			// This will check if we need to stop the thread,
			// because of a reconfiguration.
			select {
			case <-communication.HandleStream:
				return
			default:
			}

			if isKeyFrame {
				// Increment packets, so we know the device
				// is not blocking.
				r := communication.PackageCounter.Load().(int64)
				log.Log.Info("RTSPClient(Golibrtsp).Start(): packet size " + strconv.Itoa(len(pkt.Data)))
				communication.PackageCounter.Store((r + 1) % 1000)
				communication.LastPacketTimer.Store(time.Now().Unix())
			}
		}
	})

	// Play the stream.
	_, err = g.Client.Play(nil)
	if err != nil {
		panic(err)
	}

	return
}

// Decode a packet to an image.
func (g *Golibrtsp) DecodePacket(pkt packets.Packet) (image.YCbCr, error) {
	return image.YCbCr{}, nil
}

// Get a list of streams from the RTSP server.
func (j *Golibrtsp) GetStreams() ([]packets.Stream, error) {
	var streams []packets.Stream
	for _, stream := range j.Streams {
		streams = append(streams, stream)
	}
	return streams, nil
}

// Get a list of video streams from the RTSP server.
func (j *Golibrtsp) GetVideoStreams() ([]packets.Stream, error) {
	var videoStreams []packets.Stream
	for _, stream := range j.Streams {
		if stream.IsVideo {
			videoStreams = append(videoStreams, stream)
		}
	}
	return videoStreams, nil
}

// Get a list of audio streams from the RTSP server.
func (j *Golibrtsp) GetAudioStreams() ([]packets.Stream, error) {
	var audioStreams []packets.Stream
	for _, stream := range j.Streams {
		if stream.IsAudio {
			audioStreams = append(audioStreams, stream)
		}
	}
	return audioStreams, nil
}

// Close the connection to the RTSP server.
func (g *Golibrtsp) Close() error {
	// Close the demuxer.
	g.Client.Close()
	return nil
}
