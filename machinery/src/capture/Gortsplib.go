package capture

// #cgo pkg-config: libavcodec libavutil libswscale
// #include <libavcodec/avcodec.h>
// #include <libavutil/imgutils.h>
// #include <libswscale/swscale.h>
import "C"

import (
	"context"
	"errors"
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
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtplpcm"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpsimpleaudio"
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

	Client gortsplib.Client

	VideoH264Index        int8
	VideoH264Media        *description.Media
	VideoH264Forma        *format.H264
	VideoH264Decoder      *rtph264.Decoder
	VideoH264FrameDecoder *h264Decoder
	VideoH264DecoderMutex *sync.Mutex

	AudioLPCMIndex   int8
	AudioLPCMMedia   *description.Media
	AudioLPCMForma   *format.LPCM
	AudioLPCMDecoder *rtplpcm.Decoder

	AudioG711Index   int8
	AudioG711Media   *description.Media
	AudioG711Forma   *format.G711
	AudioG711Decoder *rtpsimpleaudio.Decoder

	AudioG711IndexBackChannel int8
	AudioG711MediaBackChannel *description.Media
	AudioG711FormaBackChannel *format.G711

	Streams []packets.Stream
}

// Connect to the RTSP server.
func (g *Golibrtsp) Connect(ctx context.Context) (err error) {

	g.Client = gortsplib.Client{
		RequestBackChannels: g.WithBackChannel,
	}

	// parse URL
	u, err := base.ParseURL(g.Url)
	if err != nil {
		log.Log.Debug("RTSPClient(Golibrtsp).Connect(): " + err.Error())
		return
	}

	// connect to the server
	err = g.Client.Start(u.Scheme, u.Host)
	if err != nil {
		log.Log.Debug("RTSPClient(Golibrtsp).Connect(): " + err.Error())
	}

	// find published medias
	desc, _, err := g.Client.Describe(u)
	if err != nil {
		log.Log.Debug("RTSPClient(Golibrtsp).Connect(): " + err.Error())
		return
	}

	// Iniatlise the mutex.
	g.VideoH264DecoderMutex = &sync.Mutex{}

	// find the H264 media and format
	var forma *format.H264
	medi := desc.FindFormat(&forma)
	g.VideoH264Media = medi
	g.VideoH264Forma = forma
	if medi == nil {
		log.Log.Debug("RTSPClient(Golibrtsp).Connect(): " + "video media not found")
	} else {
		// Get SPS from the SDP
		// Calculate the width and height of the video
		var sps h264.SPS
		err = sps.Unmarshal(forma.SPS)
		if err != nil {
			log.Log.Debug("RTSPClient(Golibrtsp).Connect(): " + err.Error())
			return
		}

		g.Streams = append(g.Streams, packets.Stream{
			Name:          forma.Codec(),
			IsVideo:       true,
			IsAudio:       false,
			SPS:           forma.SPS,
			PPS:           forma.PPS,
			Width:         sps.Width(),
			Height:        sps.Height(),
			FPS:           sps.FPS(),
			IsBackChannel: false,
		})

		// Set the index for the video
		g.VideoH264Index = int8(len(g.Streams)) - 1

		// setup RTP/H264 -> H264 decoder
		rtpDec, err := forma.CreateDecoder()
		if err != nil {
			// Something went wrong .. Do something
		}
		g.VideoH264Decoder = rtpDec

		// setup H264 -> raw frames decoder
		frameDec, err := newH264Decoder()
		if err != nil {
			// Something went wrong .. Do something
		}
		g.VideoH264FrameDecoder = frameDec

		// setup a video media
		_, err = g.Client.Setup(desc.BaseURL, medi, 0, 0)
		if err != nil {
			// Something went wrong .. Do something
		}
	}

	// Look for audio stream.
	// find the G711 media and format
	audioForma, audioMedi := FindPCMU(desc, false)
	g.AudioG711Media = audioMedi
	g.AudioG711Forma = audioForma
	if audioMedi == nil {
		log.Log.Debug("RTSPClient(Golibrtsp).Connect(): " + "audio media not found")
	} else {

		g.Streams = append(g.Streams, packets.Stream{
			Name:          "PCM_MULAW",
			IsVideo:       false,
			IsAudio:       true,
			IsBackChannel: false,
		})

		// Set the index for the audio
		g.AudioG711Index = int8(len(g.Streams)) - 1

		// create decoder
		audiortpDec, err := audioForma.CreateDecoder()
		if err != nil {
			// Something went wrong .. Do something
		}
		g.AudioG711Decoder = audiortpDec

		// setup a audio media
		_, err = g.Client.Setup(desc.BaseURL, audioMedi, 0, 0)
		if err != nil {
			// Something went wrong .. Do something
		}
	}

	// Look for audio back channel.
	if g.WithBackChannel {
		// find the LPCM media and format
		audioFormaBackChannel, audioMediBackChannel := FindPCMU(desc, true)
		g.AudioG711MediaBackChannel = audioMediBackChannel
		g.AudioG711FormaBackChannel = audioFormaBackChannel
		if audioMedi == nil {
			log.Log.Debug("RTSPClient(Golibrtsp).Connect(): " + "audio backchannel not found")
		} else {

			g.Streams = append(g.Streams, packets.Stream{
				Name:          "PCM_MULAW",
				IsVideo:       false,
				IsAudio:       true,
				IsBackChannel: true,
			})

			// Set the index for the audio
			g.AudioG711IndexBackChannel = int8(len(g.Streams)) - 1

			// setup a audio media
			_, err = g.Client.Setup(desc.BaseURL, audioMediBackChannel, 0, 0)
			if err != nil {
				// Something went wrong .. Do something
			}
		}
	}

	return
}

// Start the RTSP client, and start reading packets.
func (g *Golibrtsp) Start(ctx context.Context, queue *packets.Queue, communication *models.Communication) (err error) {
	log.Log.Debug("RTSPClient(Golibrtsp).Start(): started")

	// called when a audio RTP packet arrives
	if g.AudioG711Media != nil {
		g.Client.OnPacketRTP(g.AudioG711Media, g.AudioG711Forma, func(rtppkt *rtp.Packet) {
			// decode timestamp
			pts, ok := g.Client.PacketPTS(g.AudioG711Media, rtppkt)
			if !ok {
				log.Log.Error("RTSPClient(Golibrtsp).Start(): " + "unable to get PTS")
				return
			}

			// extract LPCM samples from RTP packets
			op, err := g.AudioG711Decoder.Decode(rtppkt)
			if err != nil {
				log.Log.Error("RTSPClient(Golibrtsp).Start(): " + err.Error())
				return
			}

			pkt := packets.Packet{
				IsKeyFrame:      false,
				Packet:          rtppkt,
				Data:            op,
				Time:            pts,
				CompositionTime: pts,
				Idx:             g.AudioG711Index,
			}
			queue.WritePacket(pkt)
		})
	}

	// called when a video RTP packet arrives
	if g.VideoH264Media != nil {
		g.Client.OnPacketRTP(g.VideoH264Media, g.VideoH264Forma, func(rtppkt *rtp.Packet) {

			// This will check if we need to stop the thread,
			// because of a reconfiguration.
			select {
			case <-communication.HandleStream:
				return
			default:
			}

			if len(rtppkt.Payload) > 0 {

				// decode timestamp
				pts, ok := g.Client.PacketPTS(g.VideoH264Media, rtppkt)
				if !ok {
					log.Log.Warning("RTSPClient(Golibrtsp).Start(): " + "unable to get PTS")
					return
				}

				// Extract access units from RTP packets
				// We need to do this, because the decoder expects a full
				// access unit. Once we have a full access unit, we can
				// decode it, and know if it's a keyframe or not.
				au, errDecode := g.VideoH264Decoder.Decode(rtppkt)
				if errDecode != nil {
					if errDecode != rtph264.ErrNonStartingPacketAndNoPrevious && errDecode != rtph264.ErrMorePacketsNeeded {
						log.Log.Warning("RTSPClient(Golibrtsp).Start(): " + errDecode.Error())
					}
					return
				}

				// We'll need to read out a few things.
				// prepend an AUD. This is required by some players
				filteredAU := [][]byte{
					{byte(h264.NALUTypeAccessUnitDelimiter), 240},
				}

				// Check if we have a keyframe.
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

				if len(filteredAU) <= 1 || (!nonIDRPresent && !idrPresent) {
					return
				}

				// Convert to packet.
				enc, err := h264.AnnexBMarshal(filteredAU)
				if err != nil {
					log.Log.Error("RTSPClient(Golibrtsp).Start(): " + err.Error())
					return
				}

				pkt := packets.Packet{
					IsKeyFrame:      idrPresent,
					Packet:          rtppkt,
					Data:            enc,
					Time:            pts,
					CompositionTime: pts,
					Idx:             g.VideoH264Index,
				}

				pkt.Data = pkt.Data[4:]
				if pkt.IsKeyFrame {
					annexbNALUStartCode := func() []byte { return []byte{0x00, 0x00, 0x00, 0x01} }
					pkt.Data = append(annexbNALUStartCode(), pkt.Data...)
					pkt.Data = append(g.VideoH264Forma.PPS, pkt.Data...)
					pkt.Data = append(annexbNALUStartCode(), pkt.Data...)
					pkt.Data = append(g.VideoH264Forma.SPS, pkt.Data...)
					pkt.Data = append(annexbNALUStartCode(), pkt.Data...)
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
	}

	// Play the stream.
	_, err = g.Client.Play(nil)
	if err != nil {
		panic(err)
	}

	return
}

// Decode a packet to an image.
func (g *Golibrtsp) DecodePacket(pkt packets.Packet) (image.YCbCr, error) {
	g.VideoH264DecoderMutex.Lock()
	img, err := g.VideoH264FrameDecoder.decode(pkt.Data)
	g.VideoH264DecoderMutex.Unlock()
	if err != nil {
		return image.YCbCr{}, err
	}
	if img.Bounds().Empty() {
		log.Log.Debug("RTSPClient(Golibrtsp).Start(): " + "empty frame")
		return image.YCbCr{}, errors.New("Empty frame")
	}
	return img, nil
}

// Decode a packet to a Gray image.
func (g *Golibrtsp) DecodePacketRaw(pkt packets.Packet) (image.Gray, error) {
	g.VideoH264DecoderMutex.Lock()
	img, err := g.VideoH264FrameDecoder.decodeRaw(pkt.Data)
	g.VideoH264DecoderMutex.Unlock()
	if err != nil {
		return image.Gray{}, err
	}
	if img.Bounds().Empty() {
		log.Log.Debug("RTSPClient(Golibrtsp).Start(): " + "empty frame")
		return image.Gray{}, errors.New("Empty frame")
	}

	// Do a deep copy of the image
	imgDeepCopy := image.NewGray(img.Bounds())
	imgDeepCopy.Stride = img.Stride
	copy(imgDeepCopy.Pix, img.Pix)

	return *imgDeepCopy, err
}

// Get a list of streams from the RTSP server.
func (j *Golibrtsp) GetStreams() ([]packets.Stream, error) {
	return j.Streams, nil
}

// Get a list of video streams from the RTSP server.
func (g *Golibrtsp) GetVideoStreams() ([]packets.Stream, error) {
	var videoStreams []packets.Stream
	for _, stream := range g.Streams {
		if stream.IsVideo {
			videoStreams = append(videoStreams, stream)
		}
	}
	return videoStreams, nil
}

// Get a list of audio streams from the RTSP server.
func (g *Golibrtsp) GetAudioStreams() ([]packets.Stream, error) {
	var audioStreams []packets.Stream
	for _, stream := range g.Streams {
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
	g.VideoH264FrameDecoder.Close()
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
	codecCtx *C.AVCodecContext
	srcFrame *C.AVFrame
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
func (d *h264Decoder) Close() {
	if d.srcFrame != nil {
		C.av_frame_free(&d.srcFrame)
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

func (d *h264Decoder) decodeRaw(nalu []byte) (image.Gray, error) {
	nalu = append([]uint8{0x00, 0x00, 0x00, 0x01}, []uint8(nalu)...)

	// send NALU to decoder
	var avPacket C.AVPacket
	avPacket.data = (*C.uint8_t)(C.CBytes(nalu))
	defer C.free(unsafe.Pointer(avPacket.data))
	avPacket.size = C.int(len(nalu))
	res := C.avcodec_send_packet(d.codecCtx, &avPacket)
	if res < 0 {
		return image.Gray{}, nil
	}

	// receive frame if available
	res = C.avcodec_receive_frame(d.codecCtx, d.srcFrame)
	if res < 0 {
		return image.Gray{}, nil
	}

	if res == 0 {
		fr := d.srcFrame
		w := int(fr.width)
		h := int(fr.height)
		ys := int(fr.linesize[0])

		return image.Gray{
			Pix:    fromCPtr(unsafe.Pointer(fr.data[0]), w*h),
			Stride: ys,
			Rect:   image.Rect(0, 0, w, h),
		}, nil
	}

	return image.Gray{}, nil
}

func fromCPtr(buf unsafe.Pointer, size int) (ret []uint8) {
	hdr := (*reflect.SliceHeader)((unsafe.Pointer(&ret)))
	hdr.Cap = size
	hdr.Len = size
	hdr.Data = uintptr(buf)
	return
}

func FindPCMU(desc *description.Session, isBackChannel bool) (*format.G711, *description.Media) {
	for _, media := range desc.Medias {
		if media.IsBackChannel == isBackChannel {
			for _, forma := range media.Formats {
				if g711, ok := forma.(*format.G711); ok {
					if g711.MULaw {
						return g711, media
					}
				}
			}
		}
	}
	return nil, nil
}
