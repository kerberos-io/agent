package video

import (
	"os"
	"testing"

	mp4ff "github.com/Eyevinn/mp4ff/mp4"
	"github.com/kerberos-io/agent/machinery/src/models"
)

// runLoopSeamScenario builds a fragmented MP4 that reproduces the loop-seam
// pattern observed in the failing virtual-rtsp recordings (e.g.
// thales_1781196512_3-138_2top_0-0-0-0_-1_30219.mp4): a steady GOP cadence, but
// at the source-MP4 loop boundary the source restarts and emits a fresh IDR far
// sooner than a normal GOP. In the real recordings the short tail GOP left just
// before that premature IDR contains a truncated inter-frame - software decoders
// conceal the missing macroblocks, but hardware decoders (macOS VideoToolbox,
// kVTVideoDecoderBadDataErr / -12909) and MSE players reject it and freeze
// playback at the seam (~10s in the original file).
//
// The fix detects the premature seam IDR and drops the truncated tail GOP that
// precedes it. The seam IDR is itself a clean random-access point, so playback
// resumes seamlessly. This scenario asserts that the tail GOP is removed -
// exactly one GOP fewer than emitted - while every healthy GOP is preserved in
// full and no two IDRs are left bunched in a fragment.
//
// gopFrames is the number of frames per GOP, so the same scenario can be
// exercised at different (configurable) camera GOP sizes. The fix derives its
// threshold from the observed keyframe cadence, so the truncated tail GOP is
// dropped regardless of GOP size.
func runLoopSeamScenario(t *testing.T, gopFrames int) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "test_loop_seam_*.mp4")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	sps := []byte{0x67, 0x42, 0xc0, 0x1e, 0xd9, 0x00, 0xa0, 0x47, 0xfe, 0xc8}
	pps := []byte{0x68, 0xce, 0x38, 0x80}
	mp4Video := NewMP4(tmpFile.Name(), [][]byte{sps}, [][]byte{pps}, nil, 60)
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
	normalGOPms := uint64(gopFrames) * frameDur
	pts := uint64(0)
	emitFrame := func(isKey bool) {
		// compositionOffset is 0: synthetic stream has no B-frames.
		mp4Video.AddSampleToTrack(v, isKey, mk(isKey), pts, 0)
		pts += frameDur
	}
	emitP := func(n int) {
		for i := 0; i < n; i++ {
			emitFrame(false)
		}
	}
	// emitGOP emits one GOP: a leading keyframe followed by gopFrames-1 P-frames.
	emitGOP := func() {
		emitFrame(true)
		emitP(gopFrames - 1)
	}

	// Several healthy GOPs to establish the cadence and fill a couple of
	// fragments, then the truncated tail GOP: a keyframe followed by only a few
	// P-frames before the source loops. This is the GOP that must be dropped.
	for g := 0; g < 9; g++ {
		emitGOP()
	}
	emitFrame(true)
	seamLead := gopFrames / 5 // tail GOP is only ~20% of a normal GOP before the loop
	if seamLead < 1 {
		seamLead = 1
	}
	emitP(seamLead)
	// Loop seam: the source recording restarts, emitting a fresh IDR far sooner
	// than the normal GOP. The short tail GOP emitted just above is the truncated
	// one that must be dropped; this seam IDR opens a fresh, healthy GOP.
	emitFrame(true)
	emitP(gopFrames - 1)
	// The recording continues with normal GOPs to the end.
	for g := 0; g < 10; g++ {
		emitGOP()
	}

	mp4Video.Close(&models.Config{Signing: &models.Signing{PrivateKey: ""}})

	f, err := os.Open(tmpFile.Name())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	parsed, err := mp4ff.DecodeFile(f)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	// After the fix, the truncated tail GOP that precedes the premature seam IDR
	// is dropped entirely (its first inter-frame is the incomplete one that
	// freezes hardware decoders), while every other GOP is preserved in full.
	//
	// 9 lead GOPs + the seam's own (healthy) GOP + 10 trailing GOPs = 20 committed
	// GOPs. The standalone "tail" keyframe and its seamLead P-frames are the
	// dropped truncated GOP, so the output must contain exactly one GOP fewer than
	// emitted and a whole number of complete GOPs.
	const committedGOPs = 9 + 1 + 10
	wantSync := committedGOPs
	wantSamples := committedGOPs * gopFrames

	// A healthy fragment only ever contains keyframes spaced ~normalGOPms apart.
	// If any fragment contains two keyframes closer than half a normal GOP, the
	// premature seam IDR was not dropped and the file will freeze on playback.
	maxBunchMs := normalGOPms / 2

	totalSamples := 0
	totalSync := 0
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
						totalSamples++
						// sample_depends_on == 2 => "does not depend on others" => IDR/sync.
						if (s.Flags>>24)&0x03 == 0x02 {
							keys = append(keys, offset)
							totalSync++
						}
						offset += uint64(s.Dur)
					}
				}
				t.Logf("gop=%dframes frag %d tfdt=%d samples_dur=%d keys@%v", gopFrames, fragIdx, tfdt, offset, keys)
				for i := 1; i < len(keys); i++ {
					gap := keys[i] - keys[i-1]
					if gap < maxBunchMs {
						t.Errorf("gop=%dframes frag %d (tfdt=%d): two IDRs only %d ms apart in same fragment (< %d) - seam was not dropped",
							gopFrames, fragIdx, tfdt, gap, maxBunchMs)
					}
				}
				fragIdx++
			}
		}
	}

	if totalSync != wantSync {
		t.Errorf("gop=%dframes: got %d keyframes in output, want %d - the truncated seam GOP was not dropped exactly once",
			gopFrames, totalSync, wantSync)
	}
	if totalSamples != wantSamples {
		t.Errorf("gop=%dframes: got %d video samples in output, want %d (= %d committed GOPs x %d frames) - the seam GOP drop removed the wrong frames",
			gopFrames, totalSamples, wantSamples, committedGOPs, gopFrames)
	}
}

// TestMP4LoopSeamDrop exercises the ~1s GOP case (30 frames @ ~33ms),
// matching the original failing recording.
func TestMP4LoopSeamDrop(t *testing.T) {
	runLoopSeamScenario(t, 30)
}

// TestMP4LoopSeamDropLargeGOP exercises a larger ~2s GOP (60 frames). The
// GOP size is configurable per camera; this guards against regressing to a
// fixed-millisecond threshold that would only work for ~1s GOPs.
func TestMP4LoopSeamDropLargeGOP(t *testing.T) {
	runLoopSeamScenario(t, 60)
}

// TestMP4LoopSeamDropShortGOP exercises a short ~0.5s GOP (15 frames),
// where a fixed ~1s threshold would misfire on every keyframe. The relative
// detection must only drop the genuine premature seam's truncated tail GOP.
func TestMP4LoopSeamDropShortGOP(t *testing.T) {
	runLoopSeamScenario(t, 15)
}
