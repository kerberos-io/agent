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
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtph265"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtplpcm"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpmpeg4audio"
	"github.com/bluenviron/gortsplib/v4/pkg/format/rtpsimpleaudio"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/pkg/codecs/mpeg4audio"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/packets"
	"github.com/pion/rtp"
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("github.com/kerberos-io/agent/machinery/src/capture")

// Implements the RTSPClient interface.
type Golibrtsp struct {
	RTSPClient
	Url string

	Client            gortsplib.Client
	VideoDecoderMutex *sync.Mutex

	VideoH264Index        int8
	VideoH264Media        *description.Media
	VideoH264Forma        *format.H264
	VideoH264Decoder      *rtph264.Decoder
	VideoH264FrameDecoder *Decoder

	VideoH265Index        int8
	VideoH265Media        *description.Media
	VideoH265Forma        *format.H265
	VideoH265Decoder      *rtph265.Decoder
	VideoH265FrameDecoder *Decoder

	AudioLPCMIndex   int8
	AudioLPCMMedia   *description.Media
	AudioLPCMForma   *format.LPCM
	AudioLPCMDecoder *rtplpcm.Decoder

	AudioG711Index   int8
	AudioG711Media   *description.Media
	AudioG711Forma   *format.G711
	AudioG711Decoder *rtplpcm.Decoder

	AudioOpusIndex   int8
	AudioOpusMedia   *description.Media
	AudioOpusForma   *format.Opus
	AudioOpusDecoder *rtpsimpleaudio.Decoder

	HasBackChannel            bool
	AudioG711IndexBackChannel int8
	AudioG711MediaBackChannel *description.Media
	AudioG711FormaBackChannel *format.G711

	AudioMPEG4Index   int8
	AudioMPEG4Media   *description.Media
	AudioMPEG4Forma   *format.MPEG4Audio
	AudioMPEG4Decoder *rtpmpeg4audio.Decoder

	Streams []packets.Stream

	// FPS calculation fields
	lastFrameTime    time.Time
	frameTimeBuffer  []time.Duration
	frameBufferSize  int
	frameBufferIndex int
	fpsMutex         sync.Mutex
}

// Init function
var H264FrameDecoder *Decoder
var H265FrameDecoder *Decoder

func init() {
	var err error
	// setup H264 -> raw frames decoder
	H264FrameDecoder, err = newDecoder("H264")
	if err != nil {
		log.Log.Error("capture.golibrtsp.init(): " + err.Error())
	}

	// setup H265 -> raw frames decoder
	H265FrameDecoder, err = newDecoder("H265")
	if err != nil {
		log.Log.Error("capture.golibrtsp.init(): " + err.Error())
	}
}

// Connect to the RTSP server.
func (g *Golibrtsp) Connect(ctx context.Context, ctxOtel context.Context) (err error) {

	_, span := tracer.Start(ctxOtel, "Connect")
	defer span.End()

	transport := gortsplib.TransportTCP
	g.Client = gortsplib.Client{
		RequestBackChannels: false,
		Transport:           &transport,
	}

	// parse URL
	u, err := base.ParseURL(g.Url)
	if err != nil {
		log.Log.Debug("capture.golibrtsp.Connect(ParseURL): " + err.Error())
		return
	}

	// connect to the server
	err = g.Client.Start(u.Scheme, u.Host)
	if err != nil {
		log.Log.Debug("capture.golibrtsp.Connect(Start): " + err.Error())
	}

	// find published medias
	desc, _, err := g.Client.Describe(u)
	if err != nil {
		log.Log.Debug("capture.golibrtsp.Connect(Describe): " + err.Error())
		return
	}

	// Initialize the mutex and FPS calculation.
	g.VideoDecoderMutex = &sync.Mutex{}
	g.initFPSCalculation()

	// find the H264 media and format
	var formaH264 *format.H264
	mediH264 := desc.FindFormat(&formaH264)
	g.VideoH264Media = mediH264
	g.VideoH264Forma = formaH264
	if mediH264 == nil {
		log.Log.Debug("capture.golibrtsp.Connect(H264): " + "video media not found")
	} else {
		// setup a video media
		_, err = g.Client.Setup(desc.BaseURL, mediH264, 0, 0)
		if err != nil {
			// Something went wrong .. Do something
			log.Log.Error("capture.golibrtsp.Connect(H264): " + err.Error())
		} else {
			// Get SPS and PPS from the SDP
			// Calculate the width and height of the video
			var sps h264.SPS
			errSPS := sps.Unmarshal(formaH264.SPS)
			// It might be that the SPS is not available yet, so we'll proceed,
			// but try to fetch it later on.
			if errSPS != nil {
				log.Log.Debug("capture.golibrtsp.Connect(H264): " + errSPS.Error())
				streamIndex := len(g.Streams)
				g.Streams = append(g.Streams, packets.Stream{
					Index:         streamIndex,
					Name:          formaH264.Codec(),
					IsVideo:       true,
					IsAudio:       false,
					SPS:           []byte{},
					PPS:           []byte{},
					Width:         0,
					Height:        0,
					FPS:           0,
					IsBackChannel: false,
				})
			} else {
				streamIndex := len(g.Streams)
				g.Streams = append(g.Streams, packets.Stream{
					Index:         streamIndex,
					Name:          formaH264.Codec(),
					IsVideo:       true,
					IsAudio:       false,
					SPS:           formaH264.SPS,
					PPS:           formaH264.PPS,
					Width:         sps.Width(),
					Height:        sps.Height(),
					FPS:           sps.FPS(),
					IsBackChannel: false,
				})
			}

			// Set the index for the video
			g.VideoH264Index = int8(len(g.Streams)) - 1

			// setup RTP/H264 -> H264 decoder
			rtpDec, err := formaH264.CreateDecoder()
			if err != nil {
				log.Log.Error("capture.golibrtsp.Connect(H264): " + err.Error())
			}
			g.VideoH264Decoder = rtpDec
			g.VideoH264FrameDecoder = H264FrameDecoder
		}
	}

	// find the H265 media and format
	var formaH265 *format.H265
	mediH265 := desc.FindFormat(&formaH265)
	g.VideoH265Media = mediH265
	g.VideoH265Forma = formaH265
	if mediH265 == nil {
		log.Log.Debug("capture.golibrtsp.Connect(H265): " + "video media not found")
	} else {
		// setup a video media
		_, err = g.Client.Setup(desc.BaseURL, mediH265, 0, 0)
		if err != nil {
			// Something went wrong .. Do something
			log.Log.Error("capture.golibrtsp.Connect(H265): " + err.Error())
		} else {
			// Get SPS from the SDP
			// Calculate the width and height of the video
			var sps h265.SPS
			err = sps.Unmarshal(formaH265.SPS)
			if err != nil {
				log.Log.Info("capture.golibrtsp.Connect(H265): " + err.Error())
				return
			}
			streamIndex := len(g.Streams)
			g.Streams = append(g.Streams, packets.Stream{
				Index:         streamIndex,
				Name:          formaH265.Codec(),
				IsVideo:       true,
				IsAudio:       false,
				SPS:           formaH265.SPS,
				PPS:           formaH265.PPS,
				VPS:           formaH265.VPS,
				Width:         sps.Width(),
				Height:        sps.Height(),
				FPS:           sps.FPS(),
				IsBackChannel: false,
			})

			// Set the index for the video
			g.VideoH265Index = int8(len(g.Streams)) - 1

			// setup RTP/H265 -> H265 decoder
			rtpDec, err := formaH265.CreateDecoder()
			if err != nil {
				log.Log.Error("capture.golibrtsp.Connect(H265): " + err.Error())
			}
			g.VideoH265Decoder = rtpDec

			g.VideoH265FrameDecoder = H265FrameDecoder
		}
	}

	// Look for audio stream.
	// find the G711 media and format
	audioForma, audioMedi := FindPCMU(desc, false)
	g.AudioG711Media = audioMedi
	g.AudioG711Forma = audioForma
	if audioMedi == nil {
		log.Log.Debug("capture.golibrtsp.Connect(G711): " + "audio media not found")
	} else {
		// setup a audio media
		_, err = g.Client.Setup(desc.BaseURL, audioMedi, 0, 0)
		if err != nil {
			// Something went wrong .. Do something
			log.Log.Error("capture.golibrtsp.Connect(G711): " + err.Error())
		} else {
			// create decoder
			audiortpDec, err := audioForma.CreateDecoder()
			if err != nil {
				// Something went wrong .. Do something
				log.Log.Error("capture.golibrtsp.Connect(G711): " + err.Error())
			} else {
				g.AudioG711Decoder = audiortpDec
				streamIndex := len(g.Streams)
				g.Streams = append(g.Streams, packets.Stream{
					Index:         streamIndex,
					Name:          "PCM_MULAW",
					IsVideo:       false,
					IsAudio:       true,
					IsBackChannel: false,
				})

				// Set the index for the audio
				g.AudioG711Index = int8(len(g.Streams)) - 1
			}
		}
	}

	// Look for audio stream.
	// find the Opus media and format
	audioFormaOpus, audioMediOpus := FindOPUS(desc, false)
	g.AudioOpusMedia = audioMediOpus
	g.AudioOpusForma = audioFormaOpus
	if audioMediOpus == nil {
		log.Log.Debug("capture.golibrtsp.Connect(Opus): " + "audio media not found")
	} else {
		// setup a audio media
		_, err = g.Client.Setup(desc.BaseURL, audioMediOpus, 0, 0)
		if err != nil {
			// Something went wrong .. Do something
			log.Log.Error("capture.golibrtsp.Connect(Opus): " + err.Error())
		} else {
			// create decoder
			audiortpDec, err := audioFormaOpus.CreateDecoder()
			if err != nil {
				// Something went wrong .. Do something
				log.Log.Error("capture.golibrtsp.Connect(Opus): " + err.Error())
			} else {
				g.AudioOpusDecoder = audiortpDec
				streamIndex := len(g.Streams)
				g.Streams = append(g.Streams, packets.Stream{
					Index:         streamIndex,
					Name:          "OPUS",
					IsVideo:       false,
					IsAudio:       true,
					IsBackChannel: false,
				})

				// Set the index for the audio
				g.AudioOpusIndex = int8(len(g.Streams)) - 1
			}
		}
	}

	// Look for audio stream.
	// find the AAC media and format
	audioFormaMPEG4, audioMediMPEG4 := FindMPEG4Audio(desc, false)
	g.AudioMPEG4Media = audioMediMPEG4
	g.AudioMPEG4Forma = audioFormaMPEG4
	if audioMediMPEG4 == nil {
		log.Log.Debug("capture.golibrtsp.Connect(MPEG4): " + "audio media not found")
	} else {
		// setup a audio media
		_, err = g.Client.Setup(desc.BaseURL, audioMediMPEG4, 0, 0)
		if err != nil {
			// Something went wrong .. Do something
			log.Log.Error("capture.golibrtsp.Connect(MPEG4): " + err.Error())
		} else {
			streamIndex := len(g.Streams)
			g.Streams = append(g.Streams, packets.Stream{
				Index:         streamIndex,
				Name:          "AAC",
				IsVideo:       false,
				IsAudio:       true,
				IsBackChannel: false,
				SampleRate:    audioFormaMPEG4.Config.SampleRate,
				Channels:      audioFormaMPEG4.Config.ChannelCount,
			})

			// Set the index for the audio
			g.AudioMPEG4Index = int8(len(g.Streams)) - 1

			// create decoder
			audiortpDec, err := audioFormaMPEG4.CreateDecoder()
			if err != nil {
				// Something went wrong .. Do something
				log.Log.Error("capture.golibrtsp.Connect(MPEG4): " + err.Error())
			}
			g.AudioMPEG4Decoder = audiortpDec

		}
	}

	return
}

func (g *Golibrtsp) ConnectBackChannel(ctx context.Context, ctxRunAgent context.Context) (err error) {

	_, span := tracer.Start(ctxRunAgent, "ConnectBackChannel")
	defer span.End()

	// Transport TCP
	transport := gortsplib.TransportTCP
	g.Client = gortsplib.Client{
		RequestBackChannels: true,
		Transport:           &transport,
	}
	// parse URL
	u, err := base.ParseURL(g.Url)
	if err != nil {
		log.Log.Error("capture.golibrtsp.ConnectBackChannel(): " + err.Error())
		return
	}

	// connect to the server
	err = g.Client.Start(u.Scheme, u.Host)
	if err != nil {
		log.Log.Error("capture.golibrtsp.ConnectBackChannel(): " + err.Error())
	}

	// find published medias
	desc, _, err := g.Client.Describe(u)
	if err != nil {
		log.Log.Error("capture.golibrtsp.ConnectBackChannel(): " + err.Error())
		return
	}

	// Look for audio back channel.
	g.HasBackChannel = false
	// find the LPCM media and format
	audioFormaBackChannel, audioMediBackChannel := FindPCMU(desc, true)
	g.AudioG711MediaBackChannel = audioMediBackChannel
	g.AudioG711FormaBackChannel = audioFormaBackChannel
	if audioMediBackChannel == nil {
		log.Log.Error("capture.golibrtsp.ConnectBackChannel(): audio backchannel not found, not a real error, however you might expect a backchannel. One of the reasons might be that the device already has an active client connected to the backchannel.")
		err = errors.New("no audio backchannel found")
	} else {
		// setup a audio media
		_, err = g.Client.Setup(desc.BaseURL, audioMediBackChannel, 0, 0)
		if err != nil {
			// Something went wrong .. Do something
			log.Log.Error("capture.golibrtsp.ConnectBackChannel(): " + err.Error())
			g.HasBackChannel = false
		} else {
			g.HasBackChannel = true
			streamIndex := len(g.Streams)
			g.Streams = append(g.Streams, packets.Stream{
				Index:         streamIndex,
				Name:          "PCM_MULAW",
				IsVideo:       false,
				IsAudio:       true,
				IsBackChannel: true,
			})
			// Set the index for the audio
			g.AudioG711IndexBackChannel = int8(len(g.Streams)) - 1
		}
	}
	return
}

// Start the RTSP client, and start reading packets.
func (g *Golibrtsp) Start(ctx context.Context, streamType string, queue *packets.Queue, configuration *models.Configuration, communication *models.Communication) (err error) {
	log.Log.Debug("capture.golibrtsp.Start(): started")

	// called when a MULAW audio RTP packet arrives
	if g.AudioG711Media != nil && g.AudioG711Forma != nil {
		g.Client.OnPacketRTP(g.AudioG711Media, g.AudioG711Forma, func(rtppkt *rtp.Packet) {
			pts, ok := g.Client.PacketPTS(g.AudioG711Media, rtppkt)
			// decode timestamp
			pts2, ok := g.Client.PacketPTS2(g.AudioG711Media, rtppkt)
			if !ok {
				log.Log.Debug("capture.golibrtsp.Start(): " + "unable to get PTS")
				return
			}

			// extract LPCM samples from RTP packets
			op, err := g.AudioG711Decoder.Decode(rtppkt)
			if err != nil {
				log.Log.Error("capture.golibrtsp.Start(): " + err.Error())
				return
			}

			pkt := packets.Packet{
				IsKeyFrame:      false,
				Packet:          rtppkt,
				Data:            op,
				Time:            pts2,
				TimeLegacy:      pts,
				CompositionTime: pts2,
				Idx:             g.AudioG711Index,
				IsVideo:         false,
				IsAudio:         true,
				Codec:           "PCM_MULAW",
			}
			queue.WritePacket(pkt)
		})
	}

	// called when a AAC audio RTP packet arrives
	if g.AudioMPEG4Media != nil && g.AudioMPEG4Forma != nil {
		g.Client.OnPacketRTP(g.AudioMPEG4Media, g.AudioMPEG4Forma, func(rtppkt *rtp.Packet) {
			// decode timestamp
			pts, ok := g.Client.PacketPTS(g.AudioMPEG4Media, rtppkt)
			pts2, ok := g.Client.PacketPTS2(g.AudioMPEG4Media, rtppkt)
			if !ok {
				log.Log.Error("capture.golibrtsp.Start(): " + "unable to get PTS")
				return
			}

			// Encode the AAC samples from RTP packets
			// extract access units from RTP packets
			aus, err := g.AudioMPEG4Decoder.Decode(rtppkt)
			if err != nil {
				log.Log.Error("capture.golibrtsp.Start(): " + err.Error())
				return
			}

			enc, err := WriteMPEG4Audio(g.AudioMPEG4Forma, aus)
			if err != nil {
				log.Log.Error("capture.golibrtsp.Start(): " + err.Error())
				return
			}

			pkt := packets.Packet{
				IsKeyFrame:      false,
				Packet:          rtppkt,
				Data:            enc,
				Time:            pts2,
				TimeLegacy:      pts,
				CompositionTime: pts2,
				Idx:             g.AudioG711Index,
				IsVideo:         false,
				IsAudio:         true,
				Codec:           "AAC",
			}
			queue.WritePacket(pkt)
		})
	}

	// called when a video RTP packet arrives for H264
	var filteredAU [][]byte
	if g.VideoH264Media != nil && g.VideoH264Forma != nil {

		//dtsExtractor := h264.NewDTSExtractor2()

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
				pts2, ok := g.Client.PacketPTS2(g.VideoH264Media, rtppkt)
				if !ok {
					log.Log.Debug("capture.golibrtsp.Start(): " + "unable to get PTS")
					return
				}

				// Extract access units from RTP packets
				// We need to do this, because the decoder expects a full
				// access unit. Once we have a full access unit, we can
				// decode it, and know if it's a keyframe or not.
				au, errDecode := g.VideoH264Decoder.Decode(rtppkt)
				if errDecode != nil {
					if errDecode != rtph264.ErrNonStartingPacketAndNoPrevious && errDecode != rtph264.ErrMorePacketsNeeded {
						log.Log.Error("capture.golibrtsp.Start(): " + errDecode.Error())
					}
					return
				}

				// We'll need to read out a few things.
				// prepend an AUD. This is required by some players
				filteredAU = [][]byte{
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
					case h264.NALUTypeSPS:
						// Read out sps
						var sps h264.SPS
						errSPS := sps.Unmarshal(nalu)
						if errSPS == nil {
							// Debug SPS information
							g.debugSPSInfo(&sps, streamType)

							// Get width
							g.Streams[g.VideoH264Index].Width = sps.Width()
							if streamType == "main" {
								configuration.Config.Capture.IPCamera.Width = sps.Width()
							} else if streamType == "sub" {
								configuration.Config.Capture.IPCamera.SubWidth = sps.Width()
							}
							// Get height
							g.Streams[g.VideoH264Index].Height = sps.Height()
							if streamType == "main" {
								configuration.Config.Capture.IPCamera.Height = sps.Height()
							} else if streamType == "sub" {
								configuration.Config.Capture.IPCamera.SubHeight = sps.Height()
							}
							// Get FPS using enhanced method
							fps := g.getEnhancedFPS(&sps, g.VideoH264Index)
							g.Streams[g.VideoH264Index].FPS = fps
							log.Log.Debug(fmt.Sprintf("capture.golibrtsp.Start(%s): Final FPS=%.2f", streamType, fps))
							g.VideoH264Forma.SPS = nalu
						}
					case h264.NALUTypePPS:
						g.VideoH264Forma.PPS = nalu
					}
					filteredAU = append(filteredAU, nalu)
				}

				if len(filteredAU) <= 1 || (!nonIDRPresent && !idrPresent) {
					return
				}

				// Convert to packet.
				enc, err := h264.AnnexBMarshal(filteredAU)
				if err != nil {
					log.Log.Error("capture.golibrtsp.Start(): " + err.Error())
					return
				}

				// Extract DTS from RTP packets
				//dts2, err := dtsExtractor.Extract(filteredAU, pts2)
				//if err != nil {
				// log.Log.Error("capture.golibrtsp.Start(): " + err.Error())
				// return
				//}

				pkt := packets.Packet{
					IsKeyFrame:      idrPresent,
					Packet:          rtppkt,
					Data:            enc,
					Time:            pts2,
					TimeLegacy:      pts,
					CompositionTime: pts2,
					Idx:             g.VideoH264Index,
					IsVideo:         true,
					IsAudio:         false,
					Codec:           "H264",
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
					if streamType == "main" {
						r := communication.PackageCounter.Load().(int64)
						log.Log.Debug("capture.golibrtsp.Start(): packet size " + strconv.Itoa(len(pkt.Data)))
						communication.PackageCounter.Store((r + 1) % 1000)
						communication.LastPacketTimer.Store(time.Now().Unix())
					} else if streamType == "sub" {
						r := communication.PackageCounterSub.Load().(int64)
						log.Log.Debug("capture.golibrtsp.Start(): packet size " + strconv.Itoa(len(pkt.Data)))
						communication.PackageCounterSub.Store((r + 1) % 1000)
						communication.LastPacketTimerSub.Store(time.Now().Unix())
					}
				}
			}

		})
	}

	// called when a video RTP packet arrives for H265
	if g.VideoH265Media != nil && g.VideoH265Forma != nil {
		g.Client.OnPacketRTP(g.VideoH265Media, g.VideoH265Forma, func(rtppkt *rtp.Packet) {

			// This will check if we need to stop the thread,
			// because of a reconfiguration.
			select {
			case <-communication.HandleStream:
				return
			default:
			}

			if len(rtppkt.Payload) > 0 {

				// decode timestamp
				pts, ok := g.Client.PacketPTS(g.VideoH265Media, rtppkt)
				pts2, ok := g.Client.PacketPTS2(g.VideoH265Media, rtppkt)
				if !ok {
					log.Log.Debug("capture.golibrtsp.Start(): " + "unable to get PTS")
					return
				}

				// Extract access units from RTP packets
				// We need to do this, because the decoder expects a full
				// access unit. Once we have a full access unit, we can
				// decode it, and know if it's a keyframe or not.
				au, errDecode := g.VideoH265Decoder.Decode(rtppkt)
				if errDecode != nil {
					if errDecode != rtph265.ErrNonStartingPacketAndNoPrevious && errDecode != rtph265.ErrMorePacketsNeeded {
						log.Log.Error("capture.golibrtsp.Start(): " + errDecode.Error())
					}
					return
				}

				filteredAU = [][]byte{
					{byte(h265.NALUType_AUD_NUT) << 1, 1, 0x50},
				}

				isRandomAccess := false
				for _, nalu := range au {
					typ := h265.NALUType((nalu[0] >> 1) & 0b111111)
					switch typ {
					/*case h265.NALUType_VPS_NUT:
					continue*/
					case h265.NALUType_SPS_NUT:
						continue
					case h265.NALUType_PPS_NUT:
						continue
					case h265.NALUType_AUD_NUT:
						continue
					case h265.NALUType_IDR_W_RADL, h265.NALUType_IDR_N_LP, h265.NALUType_CRA_NUT:
						isRandomAccess = true
					}
					filteredAU = append(filteredAU, nalu)
				}

				au = filteredAU

				if len(au) <= 1 {
					return
				}

				// add VPS, SPS and PPS before random access access unit
				if isRandomAccess {
					au = append([][]byte{
						g.VideoH265Forma.VPS,
						g.VideoH265Forma.SPS,
						g.VideoH265Forma.PPS}, au...)
				}

				enc, err := h264.AnnexBMarshal(au)
				if err != nil {
					log.Log.Error("capture.golibrtsp.Start(): " + err.Error())
					return
				}

				pkt := packets.Packet{
					IsKeyFrame:      isRandomAccess,
					Packet:          rtppkt,
					Data:            enc,
					Time:            pts2,
					TimeLegacy:      pts,
					CompositionTime: pts2,
					Idx:             g.VideoH265Index,
					IsVideo:         true,
					IsAudio:         false,
					Codec:           "H265",
				}

				queue.WritePacket(pkt)

				// This will check if we need to stop the thread,
				// because of a reconfiguration.
				select {
				case <-communication.HandleStream:
					return
				default:
				}

				if isRandomAccess {
					// Increment packets, so we know the device
					// is not blocking.
					if streamType == "main" {
						r := communication.PackageCounter.Load().(int64)
						log.Log.Debug("capture.golibrtsp.Start(): packet size " + strconv.Itoa(len(pkt.Data)))
						communication.PackageCounter.Store((r + 1) % 1000)
						communication.LastPacketTimer.Store(time.Now().Unix())
					} else if streamType == "sub" {
						r := communication.PackageCounterSub.Load().(int64)
						log.Log.Debug("capture.golibrtsp.Start(): packet size " + strconv.Itoa(len(pkt.Data)))
						communication.PackageCounterSub.Store((r + 1) % 1000)
						communication.LastPacketTimerSub.Store(time.Now().Unix())
					}
				}
			}

		})
	}

	// Wait for a second, so we can be sure the stream is playing.
	time.Sleep(1 * time.Second)
	// Play the stream.
	_, err = g.Client.Play(nil)
	if err != nil {
		log.Log.Error("capture.golibrtsp.Start(): " + err.Error())
	}

	return
}

// Start the RTSP client, and start reading packets.
func (g *Golibrtsp) StartBackChannel(ctx context.Context, ctxRunAgent context.Context) (err error) {
	log.Log.Info("capture.golibrtsp.StartBackChannel(): started")
	// Wait for a second, so we can be sure the stream is playing.
	time.Sleep(1 * time.Second)
	// Play the stream.
	_, err = g.Client.Play(nil)
	if err != nil {
		log.Log.Error("capture.golibrtsp.StartBackChannel(): " + err.Error())
	}
	return
}

func (g *Golibrtsp) WritePacket(pkt packets.Packet) error {
	if g.HasBackChannel && g.AudioG711MediaBackChannel != nil {
		err := g.Client.WritePacketRTP(g.AudioG711MediaBackChannel, pkt.Packet)
		if err != nil {
			log.Log.Debug("capture.golibrtsp.WritePacket(): " + err.Error())
			return err
		}
	}
	return nil
}

// Decode a packet to an image.
func (g *Golibrtsp) DecodePacket(pkt packets.Packet) (image.YCbCr, error) {
	var img image.YCbCr
	var err error
	g.VideoDecoderMutex.Lock()
	if len(pkt.Data) == 0 {
		err = errors.New("TSPClient(Golibrtsp).DecodePacket(): empty frame")
	} else if g.VideoH264Decoder != nil {
		img, err = g.VideoH264FrameDecoder.decode(pkt.Data)
	} else if g.VideoH265Decoder != nil {
		img, err = g.VideoH265FrameDecoder.decode(pkt.Data)
	} else {
		err = errors.New("TSPClient(Golibrtsp).DecodePacket(): no decoder found, might already be closed")
	}
	g.VideoDecoderMutex.Unlock()
	if err != nil {
		log.Log.Error("capture.golibrtsp.DecodePacket(): " + err.Error())
		return image.YCbCr{}, err
	}
	if img.Bounds().Empty() {
		log.Log.Debug("capture.golibrtsp.DecodePacket(): empty frame")
		return image.YCbCr{}, errors.New("Empty image")
	}
	return img, nil
}

// Decode a packet to a Gray image.
func (g *Golibrtsp) DecodePacketRaw(pkt packets.Packet) (image.Gray, error) {
	var img image.Gray
	var err error
	g.VideoDecoderMutex.Lock()
	if len(pkt.Data) == 0 {
		err = errors.New("capture.golibrtsp.DecodePacketRaw(): empty frame")
	} else if g.VideoH264Decoder != nil {
		img, err = g.VideoH264FrameDecoder.decodeRaw(pkt.Data)
	} else if g.VideoH265Decoder != nil {
		img, err = g.VideoH265FrameDecoder.decodeRaw(pkt.Data)
	} else {
		err = errors.New("capture.golibrtsp.DecodePacketRaw(): no decoder found, might already be closed")
	}
	g.VideoDecoderMutex.Unlock()
	if err != nil {
		log.Log.Error("capture.golibrtsp.DecodePacketRaw(): " + err.Error())
		return image.Gray{}, err
	}
	if img.Bounds().Empty() {
		log.Log.Debug("capture.golibrtsp.DecodePacketRaw(): empty image")
		return image.Gray{}, errors.New("Empty image")
	}

	// Do a deep copy of the image
	imgDeepCopy := image.NewGray(img.Bounds())
	imgDeepCopy.Stride = img.Stride
	copy(imgDeepCopy.Pix, img.Pix)

	return *imgDeepCopy, err
}

// Get a list of streams from the RTSP server.
func (g *Golibrtsp) GetStreams() ([]packets.Stream, error) {
	return g.Streams, nil
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
func (g *Golibrtsp) Close(ctxOtel context.Context) error {

	_, span := tracer.Start(ctxOtel, "Close")
	defer span.End()

	// Close the demuxer.
	g.Client.Close()

	// We will have created the decoders globally, so we don't need to close them here.

	//if g.VideoH264Decoder != nil {
	//	g.VideoH264FrameDecoder.Close()
	//}
	//if g.VideoH265FrameDecoder != nil {
	//	g.VideoH265FrameDecoder.Close()
	//}
	return nil
}

func frameData(frame *C.AVFrame) **C.uint8_t {
	return (**C.uint8_t)(unsafe.Pointer(&frame.data[0]))
}

func frameLineSize(frame *C.AVFrame) *C.int {
	return (*C.int)(unsafe.Pointer(&frame.linesize[0]))
}

// h264Decoder is a wrapper around FFmpeg's H264 decoder.
type Decoder struct {
	codecCtx *C.AVCodecContext
	srcFrame *C.AVFrame
}

// newH264Decoder allocates a new h264Decoder.
func newDecoder(codecName string) (*Decoder, error) {
	codec := C.avcodec_find_decoder(C.AV_CODEC_ID_H264)
	if codecName == "H265" {
		codec = C.avcodec_find_decoder(C.AV_CODEC_ID_H265)
	}
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

	return &Decoder{
		codecCtx: codecCtx,
		srcFrame: srcFrame,
	}, nil
}

// close closes the decoder.
func (d *Decoder) Close() {
	if d.srcFrame != nil {
		C.av_frame_free(&d.srcFrame)
	}
	C.av_frame_free(&d.srcFrame)
	C.avcodec_close(d.codecCtx)
}

func (d *Decoder) decode(nalu []byte) (image.YCbCr, error) {
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

func (d *Decoder) decodeRaw(nalu []byte) (image.Gray, error) {
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

func FindOPUS(desc *description.Session, isBackChannel bool) (*format.Opus, *description.Media) {
	for _, media := range desc.Medias {
		if media.IsBackChannel == isBackChannel {
			for _, forma := range media.Formats {
				if opus, ok := forma.(*format.Opus); ok {
					if opus.ChannelCount > 0 {
						return opus, media
					}
				}
			}
		}
	}
	return nil, nil
}

func FindMPEG4Audio(desc *description.Session, isBackChannel bool) (*format.MPEG4Audio, *description.Media) {
	for _, media := range desc.Medias {
		if media.IsBackChannel == isBackChannel {
			for _, forma := range media.Formats {
				if mpeg4, ok := forma.(*format.MPEG4Audio); ok {
					return mpeg4, media
				}
			}
		}
	}
	return nil, nil
}

// WriteMPEG4Audio writes MPEG-4 Audio access units.
func WriteMPEG4Audio(forma *format.MPEG4Audio, aus [][]byte) ([]byte, error) {
	pkts := make(mpeg4audio.ADTSPackets, len(aus))
	for i, au := range aus {
		pkts[i] = &mpeg4audio.ADTSPacket{
			Type:         mpeg4audio.ObjectType(forma.Config.Type),
			SampleRate:   forma.Config.SampleRate,
			ChannelCount: forma.Config.ChannelCount,
			AU:           au,
		}
	}
	enc, err := pkts.Marshal()
	if err != nil {
		return nil, err
	}
	return enc, nil
}

// Initialize FPS calculation buffers
func (g *Golibrtsp) initFPSCalculation() {
	g.frameBufferSize = 30 // Store last 30 frame intervals
	g.frameTimeBuffer = make([]time.Duration, g.frameBufferSize)
	g.frameBufferIndex = 0
	g.lastFrameTime = time.Time{}
}

// Calculate FPS from frame timestamps
func (g *Golibrtsp) calculateFPSFromTimestamps() float64 {
	g.fpsMutex.Lock()
	defer g.fpsMutex.Unlock()

	if g.lastFrameTime.IsZero() {
		g.lastFrameTime = time.Now()
		return 0
	}

	now := time.Now()
	interval := now.Sub(g.lastFrameTime)
	g.lastFrameTime = now

	// Store the interval
	g.frameTimeBuffer[g.frameBufferIndex] = interval
	g.frameBufferIndex = (g.frameBufferIndex + 1) % g.frameBufferSize

	// Calculate average FPS from stored intervals
	var totalInterval time.Duration
	validSamples := 0

	for _, interval := range g.frameTimeBuffer {
		if interval > 0 {
			totalInterval += interval
			validSamples++
		}
	}

	if validSamples == 0 {
		return 0
	}

	avgInterval := totalInterval / time.Duration(validSamples)
	if avgInterval == 0 {
		return 0
	}

	return float64(time.Second) / float64(avgInterval)
}

// Get enhanced FPS information from SPS with fallback
func (g *Golibrtsp) getEnhancedFPS(sps *h264.SPS, streamIndex int8) float64 {
	// First try to get FPS from SPS
	spsFPS := sps.FPS()

	// Check if SPS FPS is reasonable (between 1 and 120 fps)
	if spsFPS > 0 && spsFPS <= 120 {
		log.Log.Debug(fmt.Sprintf("capture.golibrtsp.getEnhancedFPS(): SPS FPS: %.2f", spsFPS))
		return spsFPS
	}

	// Fallback to timestamp-based calculation
	timestampFPS := g.calculateFPSFromTimestamps()
	if timestampFPS > 0 && timestampFPS <= 120 {
		log.Log.Debug(fmt.Sprintf("capture.golibrtsp.getEnhancedFPS(): Timestamp FPS: %.2f", timestampFPS))
		return timestampFPS
	}

	// Return SPS FPS even if it seems unreasonable, or default
	if spsFPS > 0 {
		return spsFPS
	}

	return 25.0 // Default fallback FPS
}

// Get detailed SPS timing information
func (g *Golibrtsp) getSPSTimingInfo(sps *h264.SPS) (hasVUI bool, timeScale uint32, numUnitsInTick uint32, fps float64) {
	// Try to get FPS from SPS
	fps = sps.FPS()

	// Note: The gortsplib SPS struct may not expose VUI parameters directly
	// but we can still work with the calculated FPS
	if fps > 0 {
		hasVUI = true
		// These are estimated values based on common patterns
		if fps == 25.0 {
			timeScale = 50
			numUnitsInTick = 1
		} else if fps == 30.0 {
			timeScale = 60
			numUnitsInTick = 1
		} else if fps == 24.0 {
			timeScale = 48
			numUnitsInTick = 1
		} else {
			// Generic calculation
			timeScale = uint32(fps * 2)
			numUnitsInTick = 1
		}
	}

	return hasVUI, timeScale, numUnitsInTick, fps
}

// Debug SPS information
func (g *Golibrtsp) debugSPSInfo(sps *h264.SPS, streamType string) {
	hasVUI, timeScale, numUnitsInTick, fps := g.getSPSTimingInfo(sps)

	log.Log.Debug(fmt.Sprintf("capture.golibrtsp.debugSPSInfo(%s): Width=%d, Height=%d",
		streamType, sps.Width(), sps.Height()))
	log.Log.Debug(fmt.Sprintf("capture.golibrtsp.debugSPSInfo(%s): HasVUI=%t, FPS=%.2f",
		streamType, hasVUI, fps))

	if hasVUI {
		log.Log.Debug(fmt.Sprintf("capture.golibrtsp.debugSPSInfo(%s): TimeScale=%d, NumUnitsInTick=%d",
			streamType, timeScale, numUnitsInTick))
	}
}
