package capture

// #cgo pkg-config: libavcodec libavutil libswscale
// #include <libavcodec/avcodec.h>
// #include <libavutil/imgutils.h>
// #include <libswscale/swscale.h>
import "C"

import (
	"context"
	"fmt"
	"image"
	"reflect"
	"strconv"
	"sync"
	"time"
	"unsafe"

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
	FrameDecoder *h264Decoder
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
	frameDec, err := newH264Decoder()
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
	}

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

	//dtsExtractor := h264.NewDTSExtractor()

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

			// decode timestamp
			pts, ok := g.Client.PacketPTS(g.Media, rtppkt)
			if !ok {
				log.Log.Error("RTSPClient(Golibrtsp).Start(): " + "unable to get PTS")
				return
			}

			// extract access units from RTP packets
			au, errDecode := g.Decoder.Decode(rtppkt)
			if errDecode != nil {
				if errDecode != rtph264.ErrNonStartingPacketAndNoPrevious && errDecode != rtph264.ErrMorePacketsNeeded {
					log.Log.Error("RTSPClient(Golibrtsp).Start(): " + errDecode.Error())
				}
				return
			}

			// We'll need to read out a few things.
			// prepend an AUD. This is required by some players
			filteredAU := [][]byte{
				{byte(h264.NALUTypeAccessUnitDelimiter), 240},
			}

			nonIDRPresent := false
			idrPresent := false
			for _, nalu := range au {
				typ := h264.NALUType(nalu[0] & 0x1F)
				switch typ {
				case h264.NALUTypeAccessUnitDelimiter:
					continue
				case h264.NALUTypeIDR:
					idrPresent = true

				case h264.NALUTypeNonIDR:
					nonIDRPresent = true
				}
				filteredAU = append(filteredAU, nalu)
			}

			au = filteredAU

			if len(au) <= 1 || (!nonIDRPresent && !idrPresent) {
				return
			}

			/*dts, errDts := dtsExtractor.Extract(au, pts)
			if errDts != nil {
				return
			}
			fmt.Println("DTS: ", dts)*/

			// Conver to packet.
			enc, err := h264.AnnexBMarshal(au)
			if err != nil {
				log.Log.Error("RTSPClient(Golibrtsp).Start(): " + err.Error())
				return
			}

			pkt := packets.Packet{
				IsKeyFrame:  idrPresent,
				Packet:      rtppkt,
				AccessUnits: au,
				Data:        enc,
				Time:        pts,
			}

			queue.WritePacket(pkt)

			// This will check if we need to stop the thread,
			// because of a reconfiguration.
			select {
			case <-communication.HandleStream:
				return
			default:
			}

			if idrPresent {
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
	img, err := g.FrameDecoder.decode(pkt.Data)
	if err != nil {
		return image.YCbCr{}, err
	}
	if img.Bounds().Empty() {
		log.Log.Debug("RTSPClient(Golibrtsp).Start(): " + "empty frame")
	}
	return img, nil
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

func frameData(frame *C.AVFrame) **C.uint8_t {
	return (**C.uint8_t)(unsafe.Pointer(&frame.data[0]))
}

func frameLineSize(frame *C.AVFrame) *C.int {
	return (*C.int)(unsafe.Pointer(&frame.linesize[0]))
}

// h264Decoder is a wrapper around FFmpeg's H264 decoder.
type h264Decoder struct {
	codecCtx    *C.AVCodecContext
	srcFrame    *C.AVFrame
	swsCtx      *C.struct_SwsContext
	dstFrame    *C.AVFrame
	dstFramePtr []uint8
}

// newH264Decoder allocates a new h264Decoder.
func newH264Decoder() (*h264Decoder, error) {
	codec := C.avcodec_find_decoder(C.AV_CODEC_ID_H264)
	if codec == nil {
		return nil, fmt.Errorf("avcodec_find_decoder() failed")
	}

	codecCtx := C.avcodec_alloc_context3(codec)
	if codecCtx == nil {
		return nil, fmt.Errorf("avcodec_alloc_context3() failed")
	}

	res := C.avcodec_open2(codecCtx, codec, nil)
	if res < 0 {
		C.avcodec_close(codecCtx)
		return nil, fmt.Errorf("avcodec_open2() failed")
	}

	srcFrame := C.av_frame_alloc()
	if srcFrame == nil {
		C.avcodec_close(codecCtx)
		return nil, fmt.Errorf("av_frame_alloc() failed")
	}

	return &h264Decoder{
		codecCtx: codecCtx,
		srcFrame: srcFrame,
	}, nil
}

// close closes the decoder.
func (d *h264Decoder) close() {
	if d.dstFrame != nil {
		C.av_frame_free(&d.dstFrame)
	}

	if d.swsCtx != nil {
		C.sws_freeContext(d.swsCtx)
	}

	C.av_frame_free(&d.srcFrame)
	C.avcodec_close(d.codecCtx)
}

func (d *h264Decoder) decode(nalu []byte) (image.YCbCr, error) {
	nalu = append([]uint8{0x00, 0x00, 0x00, 0x01}, []uint8(nalu)...)

	// send NALU to decoder
	var avPacket C.AVPacket
	avPacket.data = (*C.uint8_t)(C.CBytes(nalu))
	defer C.free(unsafe.Pointer(avPacket.data))
	avPacket.size = C.int(len(nalu))
	res := C.avcodec_send_packet(d.codecCtx, &avPacket)
	if res < 0 {
		return image.YCbCr{}, nil
	}

	// receive frame if available
	res = C.avcodec_receive_frame(d.codecCtx, d.srcFrame)
	if res < 0 {
		return image.YCbCr{}, nil
	}

	if res == 0 {
		fr := d.srcFrame
		w := int(fr.width)
		h := int(fr.height)
		ys := int(fr.linesize[0])
		cs := int(fr.linesize[1])

		return image.YCbCr{
			Y:              fromCPtr(unsafe.Pointer(fr.data[0]), ys*h),
			Cb:             fromCPtr(unsafe.Pointer(fr.data[1]), cs*h/2),
			Cr:             fromCPtr(unsafe.Pointer(fr.data[2]), cs*h/2),
			YStride:        ys,
			CStride:        cs,
			SubsampleRatio: image.YCbCrSubsampleRatio420,
			Rect:           image.Rect(0, 0, w, h),
		}, nil
	}

	return image.YCbCr{}, nil
}

func fromCPtr(buf unsafe.Pointer, size int) (ret []uint8) {
	hdr := (*reflect.SliceHeader)((unsafe.Pointer(&ret)))
	hdr.Cap = size
	hdr.Len = size
	hdr.Data = uintptr(buf)
	return
}
