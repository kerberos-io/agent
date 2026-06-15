package video

import (
	"os"
	"testing"

	mp4ff "github.com/Eyevinn/mp4ff/mp4"
	"github.com/kerberos-io/agent/machinery/src/models"
)

// TestMP4VariableGOPKeepsHealthyShortGOP reproduces the adam-drive regression:
// a variable-GOP ("smart codec") camera lengthens its keyframe interval during a
// static scene (e.g. 500ms -> 1500/2000ms) and then drops back to its normal
// 500ms cadence on motion. That normal, FULL 500ms GOP arrives much sooner than
// the immediately preceding (long, static) GOP.
//
// The previous heuristic compared the new keyframe interval against the *previous*
// interval and dropped the GOP whenever gap < previousInterval/2 — so every normal
// 500ms keyframe following a long static GOP was misclassified as a premature
// loop/restart seam and a whole healthy GOP (~15 frames) was discarded. In the
// field this silently deleted ~0.5s of video on virtually every recording from
// such cameras, producing a freeze/jump artifact.
//
// After the fix the seam check compares against the running MINIMUM cadence and
// additionally requires the buffered GOP to be genuinely truncated, so a full
// healthy GOP is always kept regardless of how long the preceding GOP was. This
// test asserts that NO frames are dropped for a pure variable-GOP stream.
func TestMP4VariableGOPKeepsHealthyShortGOP(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test_variable_gop_*.mp4")
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

	const frameDur = uint64(33)
	pts := uint64(0)
	emitFrame := func(isKey bool) {
		mp4Video.AddSampleToTrack(v, isKey, mk(isKey), pts, 0)
		pts += frameDur
	}
	// emitGOP emits a complete GOP of exactly frames frames: a leading keyframe
	// followed by frames-1 P-frames. Every GOP here is healthy and complete; only
	// its length varies, exactly as a smart-codec camera varies the GOP.
	emitGOP := func(frames int) {
		emitFrame(true)
		for i := 0; i < frames-1; i++ {
			emitFrame(false)
		}
	}

	// Normal cadence is 15 frames (~500ms). The camera then lengthens the GOP for
	// several static scenes (45 and 60 frames, ~1500ms and ~2000ms) before
	// dropping back to the normal 15-frame GOP on motion — the transition the old
	// heuristic wrongly treated as a seam. The whole sequence is then repeated to
	// cover multiple long->short transitions.
	gopLengths := []int{15, 15, 45, 15, 60, 15, 15, 45, 15, 15, 60, 15}
	totalEmittedFrames := 0
	emittedKeyframes := 0
	for _, n := range gopLengths {
		emitGOP(n)
		totalEmittedFrames += n
		emittedKeyframes++
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

	totalSamples := 0
	totalSync := 0
	for _, seg := range parsed.Segments {
		for _, fr := range seg.Fragments {
			for _, traf := range fr.Moof.Trafs {
				if traf.Tfhd.TrackID != 1 {
					continue
				}
				for _, trun := range traf.Truns {
					for _, s := range trun.Samples {
						totalSamples++
						// sample_depends_on == 2 => "does not depend on others" => IDR/sync.
						if (s.Flags>>24)&0x03 == 0x02 {
							totalSync++
						}
					}
				}
			}
		}
	}

	// Every GOP is healthy, so nothing must be dropped: all keyframes and all
	// frames must survive. A shortfall means a normal variable-GOP keyframe was
	// misclassified as a seam.
	if totalSync != emittedKeyframes {
		t.Errorf("got %d keyframes in output, want %d - a healthy variable-GOP keyframe was wrongly dropped as a seam",
			totalSync, emittedKeyframes)
	}
	if totalSamples != totalEmittedFrames {
		t.Errorf("got %d video samples in output, want %d - a healthy variable-GOP GOP was wrongly dropped as a seam",
			totalSamples, totalEmittedFrames)
	}
}
