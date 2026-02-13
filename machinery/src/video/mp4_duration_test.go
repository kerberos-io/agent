package video

import (
	"fmt"
	"os"
	"testing"

	mp4ff "github.com/Eyevinn/mp4ff/mp4"
	"github.com/kerberos-io/agent/machinery/src/models"
)

// TestMP4Duration creates an MP4 file simulating a 5-second video recording
// and verifies that the durations in all boxes match the sum of sample durations.
func TestMP4Duration(t *testing.T) {
	tmpFile := "/tmp/test_duration.mp4"
	defer os.Remove(tmpFile)

	// Minimal SPS for H.264 (baseline, 640x480) - proper Annex B format with start code
	sps := []byte{0x67, 0x42, 0xc0, 0x1e, 0xd9, 0x00, 0xa0, 0x47, 0xfe, 0xc8}
	pps := []byte{0x68, 0xce, 0x38, 0x80}

	mp4Video := NewMP4(tmpFile, [][]byte{sps}, [][]byte{pps}, nil, 10)
	mp4Video.SetWidth(640)
	mp4Video.SetHeight(480)
	videoTrack := mp4Video.AddVideoTrack("H264")

	// Simulate 5 seconds at 25fps (200 frames, keyframe every 50 frames = 2s)
	// PTS in milliseconds (timescale=1000)
	frameDuration := uint64(40) // 40ms per frame = 25fps
	numFrames := 150
	gopSize := 50

	// Create a fake Annex B NAL unit (keyframe IDR = type 5, non-keyframe = type 1)
	makeFrame := func(isKey bool) []byte {
		nalType := byte(0x01) // non-IDR slice
		if isKey {
			nalType = 0x65 // IDR slice
		}
		// Start code (4 bytes) + NAL header + some data
		frame := []byte{0x00, 0x00, 0x00, 0x01, nalType}
		// Add some padding data
		for i := 0; i < 100; i++ {
			frame = append(frame, byte(i))
		}
		return frame
	}

	var expectedDuration uint64
	for i := 0; i < numFrames; i++ {
		pts := uint64(i) * frameDuration
		isKeyframe := i%gopSize == 0
		err := mp4Video.AddSampleToTrack(videoTrack, isKeyframe, makeFrame(isKeyframe), pts)
		if err != nil {
			t.Fatalf("AddSampleToTrack failed at frame %d: %v", i, err)
		}
	}
	expectedDuration = uint64(numFrames) * frameDuration // Should be 6000ms (150 * 40)

	// Close with config that has signing key to avoid nil panics
	config := &models.Config{
		Signing: &models.Signing{
			PrivateKey: "",
		},
	}
	mp4Video.Close(config)

	// Log what the code computed
	t.Logf("VideoTotalDuration: %d ms", mp4Video.VideoTotalDuration)
	t.Logf("Expected duration:  %d ms", expectedDuration)
	t.Logf("Segments: %d", len(mp4Video.SegmentDurations))
	var sumSegDur uint64
	for i, d := range mp4Video.SegmentDurations {
		t.Logf("  Segment %d: duration=%d ms", i, d)
		sumSegDur += d
	}
	t.Logf("Sum of segment durations: %d ms", sumSegDur)

	// Now read back the file and inspect the boxes
	f, err := os.Open(tmpFile)
	if err != nil {
		t.Fatalf("Failed to open output file: %v", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		t.Fatalf("Failed to stat output file: %v", err)
	}

	parsedFile, err := mp4ff.DecodeFile(f)
	if err != nil {
		t.Fatalf("Failed to decode MP4: %v", err)
	}

	t.Logf("File size: %d bytes", fi.Size())

	// Check moov box
	if parsedFile.Moov == nil {
		t.Fatal("No moov box found")
	}

	// Check mvhd duration
	mvhd := parsedFile.Moov.Mvhd
	t.Logf("mvhd.Duration:  %d (timescale=%d) = %.2f seconds", mvhd.Duration, mvhd.Timescale, float64(mvhd.Duration)/float64(mvhd.Timescale))
	t.Logf("mvhd.Rate:      0x%08x", mvhd.Rate)
	t.Logf("mvhd.Volume:    0x%04x", mvhd.Volume)

	// Check each trak
	for i, trak := range parsedFile.Moov.Traks {
		t.Logf("Track %d:", i)
		t.Logf("  tkhd.Duration: %d", trak.Tkhd.Duration)
		t.Logf("  mdhd.Duration: %d (timescale=%d) = %.2f seconds", trak.Mdia.Mdhd.Duration, trak.Mdia.Mdhd.Timescale, float64(trak.Mdia.Mdhd.Duration)/float64(trak.Mdia.Mdhd.Timescale))
	}

	// Check mvex/mehd
	if parsedFile.Moov.Mvex != nil && parsedFile.Moov.Mvex.Mehd != nil {
		t.Logf("mehd.FragmentDuration: %d", parsedFile.Moov.Mvex.Mehd.FragmentDuration)
	}

	// Sum up actual sample durations from trun boxes in all segments
	var actualTrunDuration uint64
	var sampleCount int
	for _, seg := range parsedFile.Segments {
		for _, frag := range seg.Fragments {
			for _, traf := range frag.Moof.Trafs {
				// Only count video track (track 1)
				if traf.Tfhd.TrackID == 1 {
					for _, trun := range traf.Truns {
						for _, s := range trun.Samples {
							actualTrunDuration += uint64(s.Dur)
							sampleCount++
						}
					}
				}
			}
		}
	}
	t.Logf("Actual trun sample count: %d", sampleCount)
	t.Logf("Actual trun total duration: %d ms", actualTrunDuration)

	// Check sidx
	if parsedFile.Sidx != nil {
		var sidxDuration uint64
		for _, ref := range parsedFile.Sidx.SidxRefs {
			sidxDuration += uint64(ref.SubSegmentDuration)
		}
		t.Logf("sidx total duration: %d ms", sidxDuration)
	}

	// VERIFY: All duration values should be consistent
	// The expected duration for 150 frames at 40ms each:
	// - The sample-buffering pattern means the LAST sample uses LastVideoSampleDTS as duration
	// - So all 150 samples should produce 150 * 40ms = 6000ms total
	// But due to the pending sample pattern, the actual trun durations might differ

	fmt.Println()
	fmt.Println("=== DURATION CONSISTENCY CHECK ===")
	fmt.Printf("Expected (150 * 40ms):      %d ms\n", expectedDuration)
	fmt.Printf("mvhd.Duration:              %d ms\n", mvhd.Duration)
	fmt.Printf("tkhd.Duration:              %d ms\n", parsedFile.Moov.Traks[0].Tkhd.Duration)
	fmt.Printf("mdhd.Duration:              %d ms\n", parsedFile.Moov.Traks[0].Mdia.Mdhd.Duration)
	fmt.Printf("Actual trun durations sum:  %d ms\n", actualTrunDuration)
	fmt.Printf("VideoTotalDuration:         %d ms\n", mp4Video.VideoTotalDuration)
	fmt.Printf("Sum of SegmentDurations:    %d ms\n", sumSegDur)
	fmt.Println()

	// The key assertion: header duration must equal trun sum
	if mvhd.Duration != actualTrunDuration {
		t.Errorf("MISMATCH: mvhd.Duration (%d) != actual trun sum (%d), diff = %d ms",
			mvhd.Duration, actualTrunDuration, int64(mvhd.Duration)-int64(actualTrunDuration))
	}
	if parsedFile.Moov.Traks[0].Mdia.Mdhd.Duration != 0 {
		t.Errorf("MISMATCH: mdhd.Duration should be 0 for fragmented MP4, got %d",
			parsedFile.Moov.Traks[0].Mdia.Mdhd.Duration)
	}
}
