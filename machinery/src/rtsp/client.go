package rtsp

import (
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/bluenviron/gortsplib/v3"
	"github.com/bluenviron/gortsplib/v3/pkg/base"
	"github.com/bluenviron/gortsplib/v3/pkg/formats"
	"github.com/bluenviron/gortsplib/v3/pkg/formats/rtph265"
	"github.com/bluenviron/gortsplib/v3/pkg/url"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"

	"github.com/pion/rtp"
)

func CreateClient() {
	c := &gortsplib.Client{
		OnRequest: func(req *base.Request) {
			//log.Log.Info(logger.Debug, "c->s %v", req)
		},
		OnResponse: func(res *base.Response) {
			//s.Log(logger.Debug, "s->c %v", res)
		},
		OnTransportSwitch: func(err error) {
			//s.Log(logger.Warn, err.Error())
		},
		OnPacketLost: func(err error) {
			//s.Log(logger.Warn, err.Error())
		},
		OnDecodeError: func(err error) {
			//s.Log(logger.Warn, err.Error())
		},
	}

	u, err := url.Parse("rtsp://admin:admin@192.168.1.111") //"rtsp://seing:bud-edPTQc@109.159.199.103:554/rtsp/defaultPrimary?mtu=1440&streamType=m") //
	if err != nil {
		panic(err)
	}

	err = c.Start(u.Scheme, u.Host)
	if err != nil {
		//return err
	}
	defer c.Close()

	medias, baseURL, _, err := c.Describe(u)
	if err != nil {
		//return err
	}
	fmt.Println(medias)

	// find the H264 media and format
	var forma *formats.H265
	medi := medias.FindFormat(&forma)
	if medi == nil {
		panic("media not found")
	}

	// setup RTP/H264 -> H264 decoder
	rtpDec := forma.CreateDecoder()
	// setup H264 -> MPEG-TS muxer
	//pegtsMuxer, err := newMPEGTSMuxer(forma.SPS, forma.PPS)
	if err != nil {
		panic(err)
	}

	// setup H264 -> raw frames decoder
	/*h264RawDec, err := newH264Decoder()
	if err != nil {
		panic(err)
	}
	defer h264RawDec.close()

	// if SPS and PPS are present into the SDP, send them to the decoder
	if forma.SPS != nil {
		h264RawDec.decode(forma.SPS)
	}
	if forma.PPS != nil {
		h264RawDec.decode(forma.PPS)
	}*/

	readErr := make(chan error)
	go func() {
		readErr <- func() error {
			// Get codecs
			for _, medi := range medias {
				for _, forma := range medi.Formats {
					fmt.Println(forma)
				}
			}

			err = c.SetupAll(medias, baseURL)
			if err != nil {
				return err
			}

			for _, medi := range medias {
				for _, forma := range medi.Formats {
					c.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {

						au, pts, err := rtpDec.Decode(pkt)
						if err != nil {
							if err != rtph265.ErrNonStartingPacketAndNoPrevious && err != rtph265.ErrMorePacketsNeeded {
								log.Printf("ERR: %v", err)
							}
							return
						}

						for _, nalu := range au {
							log.Printf("received NALU with PTS %v and size %d\n", pts, len(nalu))
						}

						/*// extract access unit from RTP packets
						// DecodeUntilMarker is necessary for the DTS extractor to work
						if pkt.PayloadType == 96 {
							au, pts, err := rtpDec.DecodeUntilMarker(pkt)

							if err != nil {
								if err != rtph264.ErrNonStartingPacketAndNoPrevious && err != rtph264.ErrMorePacketsNeeded {
									log.Printf("ERR: %v", err)
								}
								return
							}

							// encode the access unit into MPEG-TS
							mpegtsMuxer.encode(au, pts)

							for _, nalu := range au {
								// convert NALUs into RGBA frames
								img, err := h264RawDec.decode(nalu)
								if err != nil {
									panic(err)
								}

								// wait for a frame
								if img == nil {
									continue
								}

								// convert frame to JPEG and save to file
								err = saveToFile(img)
								if err != nil {
									panic(err)
								}
							}
						}*/

					})
				}
			}

			_, err = c.Play(nil)
			if err != nil {
				return err
			}

			return c.Wait()
		}()
	}()

	for {
		select {
		case err := <-readErr:
			fmt.Println(err)
		}
	}
}

func saveToFile(img image.Image) error {
	// create file
	fname := strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10) + ".jpg"
	f, err := os.Create(fname)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	log.Println("saving", fname)

	// convert to jpeg
	return jpeg.Encode(f, img, &jpeg.Options{
		Quality: 60,
	})
}

// extract SPS and PPS without decoding RTP packets
func rtpH264ExtractSPSPPS(pkt *rtp.Packet) ([]byte, []byte) {
	if len(pkt.Payload) < 1 {
		return nil, nil
	}

	typ := h264.NALUType(pkt.Payload[0] & 0x1F)

	switch typ {
	case h264.NALUTypeSPS:
		return pkt.Payload, nil

	case h264.NALUTypePPS:
		return nil, pkt.Payload

	case h264.NALUTypeSTAPA:
		payload := pkt.Payload[1:]
		var sps []byte
		var pps []byte

		for len(payload) > 0 {
			if len(payload) < 2 {
				break
			}

			size := uint16(payload[0])<<8 | uint16(payload[1])
			payload = payload[2:]

			if size == 0 {
				break
			}

			if int(size) > len(payload) {
				return nil, nil
			}

			nalu := payload[:size]
			payload = payload[size:]

			typ = h264.NALUType(nalu[0] & 0x1F)

			switch typ {
			case h264.NALUTypeSPS:
				sps = nalu

			case h264.NALUTypePPS:
				pps = nalu
			}
		}

		return sps, pps

	default:
		return nil, nil
	}
}
