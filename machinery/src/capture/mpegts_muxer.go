package capture

import (
	"bufio"
	"fmt"
	"os"
	"sync"

	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/pkg/formats/mpegts"
	"github.com/kerberos-io/agent/machinery/src/packets"
)

func multiplyAndDivide(v, m, d int64) int64 {
	secs := v / d
	dec := v % d
	return (secs*m + dec*m/d)
}

// mpegtsMuxer allows to save a H264 / MPEG-4 audio stream into a MPEG-TS file.
type MpegtsMuxer struct {
	FileName         string
	IsOpen           bool
	H264Format       *format.H264
	Mpeg4AudioFormat *format.MPEG4Audio

	f               *os.File
	b               *bufio.Writer
	w               *mpegts.Writer
	H264Track       *mpegts.Track
	Mpeg4AudioTrack *mpegts.Track
	dtsExtractor    *h264.DTSExtractor2
	mutex           sync.Mutex
}

// initialize initializes a mpegtsMuxer.
func (e *MpegtsMuxer) Initialize() error {
	var err error
	e.f, err = os.Create(e.FileName)
	if err != nil {
		return err
	}
	e.IsOpen = true
	e.b = bufio.NewWriter(e.f)

	e.H264Track = &mpegts.Track{
		Codec: &mpegts.CodecH264{},
	}

	e.dtsExtractor = h264.NewDTSExtractor2()

	/*e.Mpeg4AudioTrack = &mpegts.Track{
		Codec: &mpegts.CodecMPEG4Audio{
			Config: *e.Mpeg4AudioFormat.Config,
		},
	}*/

	e.w = mpegts.NewWriter(e.b, []*mpegts.Track{e.H264Track}) //, e.Mpeg4AudioTrack})

	return nil
}

// close closes all the mpegtsMuxer resources.
func (e *MpegtsMuxer) Close() {
	e.IsOpen = false
	e.b.Flush()
	e.f.Close()
}

// writeH264 writes a H264 access unit into MPEG-TS.
func (e *MpegtsMuxer) WriteH264(pkt packets.Packet) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	au := pkt.OrginialAU

	if au == nil || pkt.IsAudio {
		return nil
	}

	var filteredAU [][]byte

	nonIDRPresent := false
	idrPresent := false

	var SPS []byte
	var PPS []byte
	for _, nalu := range au {
		typ := h264.NALUType(nalu[0] & 0x1F)
		switch typ {
		case h264.NALUTypeSPS:
			SPS = nalu
			continue

		case h264.NALUTypePPS:
			PPS = nalu
			continue

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

	if au == nil || (!nonIDRPresent && !idrPresent) {
		return nil
	}

	// add SPS and PPS before access unit that contains an IDR
	if idrPresent {
		au = append([][]byte{SPS, PPS}, au...)
	}

	dts, err := e.dtsExtractor.Extract(au, pkt.Time)
	if err != nil {
		fmt.Println("Error extracting DTS: ", err)
		//return err
	}
	return e.w.WriteH264(e.H264Track, pkt.Time, dts, pkt.IsKeyFrame, au)
}

// writeMPEG4Audio writes MPEG-4 audio access units into MPEG-TS.
func (e *MpegtsMuxer) WriteMPEG4Audio(aus [][]byte, pts int64) error {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	return e.w.WriteMPEG4Audio(e.Mpeg4AudioTrack, multiplyAndDivide(pts, 90000, int64(e.Mpeg4AudioFormat.ClockRate())), aus)
}
