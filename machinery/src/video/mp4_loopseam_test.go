package video

import (
	"fmt"
	"os"
	"testing"

	mp4ff "github.com/Eyevinn/mp4ff/mp4"
	"github.com/kerberos-io/agent/machinery/src/models"
)

// TestMP4LoopSeamIsolation reproduces the loop-seam pattern from the
// failing virtual-rtsp recordings: ~1s GOPs, but at the source-MP4
// loop boundary an IDR arrives prematurely (~200-870ms after the
// previous IDR). Without the fix this seam IDR ends up bunched into
// the same fragment as the prior GOP's IDR which trips macOS
// VideoToolbox (kVTVideoDecoderBadDataErr / -12909). The fix forces
// a fragment flush whenever two IDRs arrive closer than MinNormalGOPMs.
func TestMP4LoopSeamIsolation(t *testing.T) {
	tmpFile := "/tmp/test_loop_seam.mp4"
	defer os.Remove(tmpFile)

	sps := []byte{0x67, 0x42, 0xc0, 0x1e, 0xd9, 0x00, 0xa0, 0x47, 0xfe, 0xc8}
	pps := []byte{0x68, 0xce, 0x38, 0x80}
	mp4Video := NewMP4(tmpFile, [][]byte{sps}, [][]byte{pps}, nil, 30)
	mp4Video.SetWidth(1920)
	mp4Video.SetHeight(1080)
	v := mp4Video.AddVideoTrack("H264")

	mk := func(k bool) []byte {
		nt := byte(0x01)
		if k {
			nt = 0x65
		}
		f := []byte{0, 0, 0, 1, nt}
		for i := 0; i < 200; i++ {
			f = append(f, byte(i))
		}
		return f
	}

	frameDur := uint64(33)
	pts := uint64(0)
	emit := func(n int, gopLen int) {
		for f := 0; f < n; f++ {
			isKey := (f % gopLen) == 0
			mp4Video.AddSampleToTrack(v, isKey, mk(isKey), pts)
			pts += frameDur
		}
	}

	// 17 seconds of normal content (last "good" IDR at sec 17).
	emit(17*30, 30)
	// Seam: IDR arrives ~867ms after previous (vs normal 1000ms).
	pts -= 100
	emit(13*30, 30)

	mp4Video.Close(&models.Config{Signing: &models.Signing{PrivateKey: ""}})

	f, _ := os.Open(tmpFile)
	defer f.Close()
	parsed, err := mp4ff.DecodeFile(f)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	fragIdx := 0
	for _, seg := range parsed.Segments {
		for _, fr := range seg.Fragments {
			for _, traf := range fr.Moof.Trafs {
				if traf.Tfhd.TrackID != 1 {
					continue
				}
				tfdt := traf.Tfdt.BaseMediaDecodeTime()
				offset := uint64(0)
				var keys []uint64
				for _, trun := range traf.Truns {
					for _, s := range trun.Samples {
						if (s.Flags>>24)&0x03 == 0x02 {
							keys = append(keys, offset)
						}
						offset += uint64(s.Dur)
					}
				}
				fmt.Printf("frag %d tfdt=%d samples_dur=%d keys@%v\n",
					fragIdx, tfdt, offset, keys)
				for i := 1; i < len(keys); i++ {
					gap := keys[i] - keys[i-1]
					if gap < MinNormalGOPMs {
						t.Errorf("frag %d (tfdt=%d): two IDRs only %d ms apart "+
							"in same fragment (< %d) - seam was not isolated",
							fragIdx, tfdt, gap, MinNormalGOPMs)
					}
				}
				fragIdx++
			}
		}
	}
}
