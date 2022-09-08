package components

import (
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"time"

	"github.com/deepch/vdk/av"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/format/rtsp"
	"github.com/nsmith5/mjpeg"
)

type Stream struct {
	Name   string
	Url    string
	Debug  bool
	Codecs string
}

func CreateStream(name string, url string) *Stream {
	return &Stream{
		Name: name,
		Url:  url,
	}
}

func (s Stream) Open() *rtsp.Client {

	// Enable debugging
	if s.Debug {
		rtsp.DebugRtsp = true
	}

	fmt.Println("Dialing in to " + s.Url)
	session, err := rtsp.Dial(s.Url)
	if err != nil {
		log.Println("Something went wrong dialing into stream: ", err)
		time.Sleep(5 * time.Second)
	}
	session.RtpKeepAliveTimeout = 10 * time.Second
	return session
}

func (s Stream) Close(session *rtsp.Client) {
	fmt.Println("Closing RTSP session.")
	err := session.Close()
	if err != nil {
		log.Println("Something went wrong while closing your RTSP session: ", err)
	}
}

func (s Stream) GetCodecs() []av.CodecData {
	session := s.Open()
	codec, err := session.Streams()
	log.Println("Reading codecs from stream: ", codec)
	if err != nil {
		log.Println("Something went wrong while reading codecs from stream: ", err)
		time.Sleep(5 * time.Second)
	}
	s.Close(session)
	return codec
}

func (s Stream) ReadPackets(packetChannel chan av.Packet) {
	session := s.Open()
	fmt.Println("Start reading H264 packages from stream")
	for {
		packet, err := session.ReadPacket()
		if err != nil {
			break
		}
		if len(packetChannel) < cap(packetChannel) {
			packetChannel <- packet
		}
	}
	s.Close(session)
}

func GetSPSFromCodec(codecs []av.CodecData) ([]byte, []byte) {
	sps := codecs[0].(h264parser.CodecData).SPS()
	pps := codecs[0].(h264parser.CodecData).PPS()
	return sps, pps
}

func StartMotionJPEG(imageFunction func() (image.Image, error), quality int) mjpeg.Handler {
	stream := mjpeg.Handler{
		Next:    imageFunction,
		Options: &jpeg.Options{Quality: quality},
	}
	return stream
}
