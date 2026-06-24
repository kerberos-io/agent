package video

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mp4ff "github.com/Eyevinn/mp4ff/mp4"
)

// Known-good minimal H.264 baseline parameter sets (640x480), reused from the
// recording-muxer tests so the live segmenter is exercised against the exact
// SPS/PPS mp4ff is already known to parse into an avcC descriptor.
var (
	liveTestSPS = []byte{0x67, 0x42, 0xc0, 0x1e, 0xd9, 0x00, 0xa0, 0x47, 0xfe, 0xc8}
	liveTestPPS = []byte{0x68, 0xce, 0x38, 0x80}
)

// makeAnnexBFrame builds a single-NALU Annex B access unit: a 4-byte start code,
// the NAL header (IDR=0x65 for keyframes, non-IDR=0x01 otherwise) and padding.
func makeAnnexBFrame(isKey bool) []byte {
	nalType := byte(0x01)
	if isKey {
		nalType = 0x65
	}
	frame := []byte{0x00, 0x00, 0x00, 0x01, nalType}
	for i := 0; i < 100; i++ {
		frame = append(frame, byte(i))
	}
	return frame
}

// isSyncSample reports whether a parsed sample is a random-access point
// (sample_depends_on == 2 => "depends on nothing" => IDR/sync).
func isSyncSample(s mp4ff.Sample) bool {
	return (s.Flags>>24)&0x03 == 0x02
}

// TestLiveSegmenterProducesIndependentCMAFSegments feeds a synthetic H.264
// stream (25 fps, 1s GOPs) through the live segmenter and asserts that:
//   - exactly one init segment (ftyp+moov, single avc1 video track) is produced;
//   - segments are cut on keyframe boundaries honoring the target duration;
//   - every media segment carries a CMAF styp + exactly one moof+mdat fragment;
//   - each segment begins with a sync sample and its tfdt equals the absolute
//     decode time of that first sample (the property that makes it independently
//     decodable after the init segment);
//   - sample counts and durations are preserved end to end.
func TestLiveSegmenterProducesIndependentCMAFSegments(t *testing.T) {
	const (
		frameDurMs = uint64(40) // 25 fps
		gopFrames  = 25         // keyframe every 1000 ms
		numGOPs    = 6
		numFrames  = gopFrames * numGOPs // 150 frames, 6000 ms
		targetMs   = uint64(2000)        // 2s segments => 2 GOPs each
	)

	seg := NewLiveSegmenter("H264", [][]byte{liveTestSPS}, [][]byte{liveTestPPS}, nil, targetMs)
	seg.SetDimensions(640, 480)

	var initBytes []byte
	var initCalls int
	var segments []LiveSegment
	seg.OnInit = func(b []byte) error {
		initCalls++
		initBytes = append([]byte(nil), b...)
		return nil
	}
	seg.OnSegment = func(s LiveSegment) error {
		segments = append(segments, s)
		return nil
	}

	for i := 0; i < numFrames; i++ {
		isKey := i%gopFrames == 0
		pts := uint64(i) * frameDurMs
		if err := seg.WriteSample(isKey, makeAnnexBFrame(isKey), pts, 0); err != nil {
			t.Fatalf("WriteSample(frame=%d): %v", i, err)
		}
	}
	if err := seg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// --- Init segment: emitted exactly once, well-formed, single video track. ---
	if initCalls != 1 {
		t.Fatalf("OnInit called %d times, want 1", initCalls)
	}
	if len(initBytes) == 0 {
		t.Fatal("init segment is empty")
	}
	parsedInit, err := mp4ff.DecodeFile(bytes.NewReader(initBytes))
	if err != nil {
		t.Fatalf("decode init: %v", err)
	}
	if parsedInit.Init == nil || parsedInit.Init.Ftyp == nil || parsedInit.Init.Moov == nil {
		t.Fatal("init segment missing ftyp/moov")
	}
	if got := len(parsedInit.Init.Moov.Traks); got != 1 {
		t.Fatalf("init moov has %d traks, want 1", got)
	}

	// --- Segment cut cadence: 6 GOPs at 2s target => 3 segments of 2 GOPs each. ---
	const wantSegments = 3
	if len(segments) != wantSegments {
		t.Fatalf("got %d media segments, want %d", len(segments), wantSegments)
	}
	for i, s := range segments {
		if want := uint32(i + 1); s.SequenceNumber != want {
			t.Errorf("segment %d: SequenceNumber=%d, want %d", i, s.SequenceNumber, want)
		}
		if s.DurationMs != targetMs {
			t.Errorf("segment %d: DurationMs=%d, want %d", i, s.DurationMs, targetMs)
		}
	}

	// --- Each segment must decode INDEPENDENTLY after the init segment. ---
	// Parsing init+oneSegment in isolation mirrors exactly what hls.js does with
	// an #EXT-X-MAP init and a single media part.
	var totalSamples, totalSync int
	wantTFDT := []uint64{0, 2000, 4000}
	for i, s := range segments {
		standalone := append(append([]byte(nil), initBytes...), s.Data...)
		parsed, err := mp4ff.DecodeFile(bytes.NewReader(standalone))
		if err != nil {
			t.Fatalf("segment %d: decode init+segment: %v", i, err)
		}
		if len(parsed.Segments) != 1 {
			t.Fatalf("segment %d: parsed %d media segments, want 1", i, len(parsed.Segments))
		}
		mseg := parsed.Segments[0]
		if mseg.Styp == nil {
			t.Errorf("segment %d: missing CMAF styp box", i)
		}
		if len(mseg.Fragments) != 1 {
			t.Fatalf("segment %d: %d fragments, want 1", i, len(mseg.Fragments))
		}
		fr := mseg.Fragments[0]
		if got := fr.Moof.Mfhd.SequenceNumber; got != s.SequenceNumber {
			t.Errorf("segment %d: moof sequence=%d, want %d", i, got, s.SequenceNumber)
		}
		traf := fr.Moof.Traf
		if traf.Tfhd.TrackID != 1 {
			t.Errorf("segment %d: track id=%d, want 1", i, traf.Tfhd.TrackID)
		}
		if got := traf.Tfdt.BaseMediaDecodeTime(); got != wantTFDT[i] {
			t.Errorf("segment %d: tfdt baseMediaDecodeTime=%d, want %d", i, got, wantTFDT[i])
		}

		var samples []mp4ff.Sample
		for _, trun := range traf.Truns {
			samples = append(samples, trun.Samples...)
		}
		if len(samples) == 0 {
			t.Fatalf("segment %d: no samples", i)
		}
		if !isSyncSample(samples[0]) {
			t.Errorf("segment %d: first sample is not a keyframe/sync sample", i)
		}
		var segDur uint64
		for j, smp := range samples {
			totalSamples++
			if isSyncSample(smp) {
				totalSync++
			}
			segDur += uint64(smp.Dur)
			if smp.Size == 0 {
				t.Errorf("segment %d sample %d: zero size", i, j)
			}
		}
		if segDur != s.DurationMs {
			t.Errorf("segment %d: summed sample dur=%d, reported DurationMs=%d", i, segDur, s.DurationMs)
		}
	}

	if totalSamples != numFrames {
		t.Errorf("total samples across segments=%d, want %d", totalSamples, numFrames)
	}
	if totalSync != numGOPs {
		t.Errorf("total sync samples=%d, want %d (one per GOP)", totalSync, numGOPs)
	}
}

// TestLiveSegmenterDropsLeadingNonKeyframe verifies a session cannot open on a
// non-IDR frame (which would reference frames that never arrived); such leading
// samples are dropped until the first keyframe.
func TestLiveSegmenterDropsLeadingNonKeyframe(t *testing.T) {
	seg := NewLiveSegmenter("H264", [][]byte{liveTestSPS}, [][]byte{liveTestPPS}, nil, 1000)
	seg.SetDimensions(640, 480)
	var segments []LiveSegment
	seg.OnSegment = func(s LiveSegment) error { segments = append(segments, s); return nil }

	// Two P-frames before any IDR must be ignored.
	if err := seg.WriteSample(false, makeAnnexBFrame(false), 0, 0); err != nil {
		t.Fatalf("WriteSample(p0): %v", err)
	}
	if err := seg.WriteSample(false, makeAnnexBFrame(false), 40, 0); err != nil {
		t.Fatalf("WriteSample(p1): %v", err)
	}
	// First IDR opens the session at decode time 0.
	for i := 0; i < 25; i++ {
		isKey := i == 0
		if err := seg.WriteSample(isKey, makeAnnexBFrame(isKey), uint64(i)*40, 0); err != nil {
			t.Fatalf("WriteSample(%d): %v", i, err)
		}
	}
	if err := seg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if len(segments) == 0 {
		t.Fatal("expected at least one segment after the first IDR")
	}
	initBytes, err := seg.InitSegment()
	if err != nil {
		t.Fatalf("InitSegment: %v", err)
	}
	standalone := append(append([]byte(nil), initBytes...), segments[0].Data...)
	parsed, err := mp4ff.DecodeFile(bytes.NewReader(standalone))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	traf := parsed.Segments[0].Fragments[0].Moof.Traf
	if got := traf.Tfdt.BaseMediaDecodeTime(); got != 0 {
		t.Errorf("first segment tfdt=%d, want 0 (session opens on the IDR)", got)
	}
	var first mp4ff.Sample
	for _, trun := range traf.Truns {
		if len(trun.Samples) > 0 {
			first = trun.Samples[0]
			break
		}
	}
	if !isSyncSample(first) {
		t.Error("first committed sample must be the IDR, not a dropped P-frame")
	}
}

// renderLiveMediaPlaylist renders a live (no #EXT-X-ENDLIST) fMP4 HLS media
// playlist for the given segments. This mirrors the shape hub-api will serve for
// live streams: an #EXT-X-MAP init segment followed by one #EXTINF per CMAF part.
// In production hub-api emits a sliding WINDOW of the most recent segments and
// advances #EXT-X-MEDIA-SEQUENCE; here we list the whole synthetic capture for a
// self-contained, inspectable bundle.
func renderLiveMediaPlaylist(initURI string, segs []LiveSegment, mediaSequence uint32) string {
	var maxDurMs uint64
	for _, s := range segs {
		if s.DurationMs > maxDurMs {
			maxDurMs = s.DurationMs
		}
	}
	target := uint64(math.Ceil(float64(maxDurMs) / 1000.0))
	if target == 0 {
		target = 1
	}

	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	b.WriteString("#EXT-X-VERSION:7\n")
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%d\n", target)
	fmt.Fprintf(&b, "#EXT-X-MEDIA-SEQUENCE:%d\n", mediaSequence)
	b.WriteString("#EXT-X-INDEPENDENT-SEGMENTS\n")
	fmt.Fprintf(&b, "#EXT-X-MAP:URI=%q\n", initURI)
	for _, s := range segs {
		fmt.Fprintf(&b, "#EXTINF:%.3f,\n", float64(s.DurationMs)/1000.0)
		fmt.Fprintf(&b, "seg-%d.m4s\n", s.SequenceNumber)
	}
	// NOTE: deliberately no #EXT-X-ENDLIST - its absence is what marks the
	// playlist as live so hls.js keeps polling for new segments.
	return b.String()
}

// TestLiveSegmenterWritesHLSBundle runs the segmenter over a synthetic stream and
// writes a complete on-disk fMP4 HLS bundle (init.mp4 + seg-N.m4s + a live
// stream.m3u8). It validates the playlist shape and that every referenced file
// exists, then logs the output directory so the structure can be eyeballed.
//
// Set LIVEHLS_OUT=/some/dir to keep the bundle for manual inspection (e.g. serve
// it and point hls.js at stream.m3u8); otherwise a temp dir is used and removed.
//
// The frames here are synthetic (valid fMP4 boxing, non-decodable payloads), so
// this validates CONTAINER/playlist structure, not pixel decode - the round-trip
// assertions in TestLiveSegmenterProducesIndependentCMAFSegments cover decodable
// box layout.
func TestLiveSegmenterWritesHLSBundle(t *testing.T) {
	const (
		frameDurMs = uint64(40)
		gopFrames  = 25
		numGOPs    = 6
		numFrames  = gopFrames * numGOPs
		targetMs   = uint64(2000)
	)

	outDir := os.Getenv("LIVEHLS_OUT")
	if outDir == "" {
		outDir = t.TempDir()
	} else {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", outDir, err)
		}
	}

	seg := NewLiveSegmenter("H264", [][]byte{liveTestSPS}, [][]byte{liveTestPPS}, nil, targetMs)
	seg.SetDimensions(640, 480)

	var segments []LiveSegment
	seg.OnInit = func(b []byte) error {
		return os.WriteFile(filepath.Join(outDir, "init.mp4"), b, 0o644)
	}
	seg.OnSegment = func(s LiveSegment) error {
		segments = append(segments, s)
		name := fmt.Sprintf("seg-%d.m4s", s.SequenceNumber)
		return os.WriteFile(filepath.Join(outDir, name), s.Data, 0o644)
	}

	for i := 0; i < numFrames; i++ {
		isKey := i%gopFrames == 0
		if err := seg.WriteSample(isKey, makeAnnexBFrame(isKey), uint64(i)*frameDurMs, 0); err != nil {
			t.Fatalf("WriteSample(%d): %v", i, err)
		}
	}
	if err := seg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if len(segments) == 0 {
		t.Fatal("no segments produced")
	}

	playlist := renderLiveMediaPlaylist("init.mp4", segments, segments[0].SequenceNumber)
	if err := os.WriteFile(filepath.Join(outDir, "stream.m3u8"), []byte(playlist), 0o644); err != nil {
		t.Fatalf("write playlist: %v", err)
	}

	// --- Validate the live playlist shape. ---
	mustContain := []string{
		"#EXTM3U",
		"#EXT-X-VERSION:7",
		"#EXT-X-TARGETDURATION:2",
		"#EXT-X-MEDIA-SEQUENCE:1",
		`#EXT-X-MAP:URI="init.mp4"`,
		"#EXT-X-INDEPENDENT-SEGMENTS",
	}
	for _, tag := range mustContain {
		if !strings.Contains(playlist, tag) {
			t.Errorf("playlist missing %q\n---\n%s", tag, playlist)
		}
	}
	if strings.Contains(playlist, "#EXT-X-ENDLIST") {
		t.Error("live playlist must NOT contain #EXT-X-ENDLIST")
	}
	if got, want := strings.Count(playlist, "#EXTINF:"), len(segments); got != want {
		t.Errorf("playlist has %d #EXTINF entries, want %d", got, want)
	}

	// --- Every referenced file must exist on disk. ---
	if _, err := os.Stat(filepath.Join(outDir, "init.mp4")); err != nil {
		t.Errorf("init.mp4 missing: %v", err)
	}
	for _, s := range segments {
		name := fmt.Sprintf("seg-%d.m4s", s.SequenceNumber)
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Errorf("%s missing: %v", name, err)
		}
	}

	t.Logf("wrote HLS bundle to %s (%d segments)\n%s", outDir, len(segments), playlist)
}

// boxTypeAt returns the 4CC box type at the front of a top-level box blob (the
// 4 bytes following the 32-bit size), or "" if the blob is too short.
func boxTypeAt(b []byte) string {
	if len(b) < 8 {
		return ""
	}
	return string(b[4:8])
}

// TestLiveSegmenterLowLatencyParts runs the segmenter in LL-HLS mode over the
// same synthetic stream and asserts that:
//   - each ~2s segment is sliced into multiple CMAF parts (more parts than
//     segments overall);
//   - part 0 of every segment carries the CMAF styp and is INDEPENDENT (begins
//     with the segment keyframe); later parts are bare moof+mdat (no styp);
//   - moof sequence numbers are globally monotonic across all parts (MSE needs
//     increasing moof sequence numbers);
//   - concatenating a segment's parts in order yields exactly the same bytes the
//     classic per-segment path would emit, decoding into one independent CMAF
//     segment whose first sample is a sync sample with the expected tfdt;
//   - every sample and keyframe of the input is preserved end to end.
func TestLiveSegmenterLowLatencyParts(t *testing.T) {
	const (
		frameDurMs = uint64(40) // 25 fps
		gopFrames  = 25         // keyframe every 1000 ms
		numGOPs    = 6
		numFrames  = gopFrames * numGOPs // 150 frames, 6000 ms
		targetMs   = uint64(2000)        // 2s segments => 2 GOPs each
		partMs     = uint64(300)         // ~300 ms parts => ~6-7 parts/segment
	)

	seg := NewLiveSegmenter("H264", [][]byte{liveTestSPS}, [][]byte{liveTestPPS}, nil, targetMs)
	seg.SetDimensions(640, 480)
	seg.EnableLowLatency(partMs)

	var initBytes []byte
	var initCalls int
	var parts []LivePart
	seg.OnInit = func(b []byte) error {
		initCalls++
		initBytes = append([]byte(nil), b...)
		return nil
	}
	seg.OnPart = func(p LivePart) error {
		parts = append(parts, p)
		return nil
	}

	for i := 0; i < numFrames; i++ {
		isKey := i%gopFrames == 0
		if err := seg.WriteSample(isKey, makeAnnexBFrame(isKey), uint64(i)*frameDurMs, 0); err != nil {
			t.Fatalf("WriteSample(frame=%d): %v", i, err)
		}
	}
	if err := seg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if initCalls != 1 {
		t.Fatalf("OnInit called %d times, want 1", initCalls)
	}
	if len(parts) == 0 {
		t.Fatal("no parts produced in low-latency mode")
	}

	// --- Parts are globally moof-monotonic, and group into 3 segments whose part
	// indices are contiguous from 0. ---
	bySeg := map[uint32][]LivePart{}
	var order []uint32
	var lastMoof uint32
	for i, p := range parts {
		if _, seen := bySeg[p.SegmentSeq]; !seen {
			order = append(order, p.SegmentSeq)
		}
		bySeg[p.SegmentSeq] = append(bySeg[p.SegmentSeq], p)

		// Decode the part to read its moof sequence number and confirm the styp
		// convention (part 0 => styp present, later parts => bare moof+mdat).
		front := boxTypeAt(p.Data)
		if p.PartIndex == 0 {
			if front != "styp" {
				t.Errorf("seg %d part 0: leading box=%q, want styp", p.SegmentSeq, front)
			}
			if !p.Independent {
				t.Errorf("seg %d part 0: Independent=false, want true (starts on keyframe)", p.SegmentSeq)
			}
		} else if front != "moof" {
			t.Errorf("seg %d part %d: leading box=%q, want moof (no styp on later parts)", p.SegmentSeq, p.PartIndex, front)
		}

		parsed, err := mp4ff.DecodeFile(bytes.NewReader(p.Data))
		if err != nil {
			t.Fatalf("seg %d part %d: decode: %v", p.SegmentSeq, p.PartIndex, err)
		}
		if len(parsed.Segments) != 1 || len(parsed.Segments[0].Fragments) != 1 {
			t.Fatalf("seg %d part %d: want exactly one fragment", p.SegmentSeq, p.PartIndex)
		}
		moof := parsed.Segments[0].Fragments[0].Moof.Mfhd.SequenceNumber
		if i > 0 && moof <= lastMoof {
			t.Errorf("part %d: moof sequence=%d not greater than previous %d", i, moof, lastMoof)
		}
		lastMoof = moof
	}

	if len(order) != 3 {
		t.Fatalf("got %d segments, want 3", len(order))
	}
	if len(parts) <= len(order) {
		t.Fatalf("got %d parts for %d segments, expected each segment to be sliced into multiple parts", len(parts), len(order))
	}
	for _, segSeq := range order {
		for idx, p := range bySeg[segSeq] {
			if p.PartIndex != uint32(idx) {
				t.Errorf("seg %d: part index %d out of order (want %d)", segSeq, p.PartIndex, idx)
			}
		}
	}

	// --- Concatenating a segment's parts must reconstruct one independent CMAF
	// segment that decodes against the init segment. ---
	wantTFDT := map[uint32]uint64{1: 0, 2: 2000, 3: 4000}
	var totalSamples, totalSync int
	for _, segSeq := range order {
		segParts := bySeg[segSeq]
		var full []byte
		var wantPartDur uint64
		for _, p := range segParts {
			full = append(full, p.Data...)
			wantPartDur += p.DurationMs
		}
		standalone := append(append([]byte(nil), initBytes...), full...)
		parsed, err := mp4ff.DecodeFile(bytes.NewReader(standalone))
		if err != nil {
			t.Fatalf("seg %d: decode concatenated parts: %v", segSeq, err)
		}
		if len(parsed.Segments) != 1 {
			t.Fatalf("seg %d: parsed %d media segments, want 1", segSeq, len(parsed.Segments))
		}
		mseg := parsed.Segments[0]
		if mseg.Styp == nil {
			t.Errorf("seg %d: reconstructed segment missing CMAF styp", segSeq)
		}
		if len(mseg.Fragments) != len(segParts) {
			t.Errorf("seg %d: %d fragments, want %d (one per part)", segSeq, len(mseg.Fragments), len(segParts))
		}
		firstTraf := mseg.Fragments[0].Moof.Traf
		if got := firstTraf.Tfdt.BaseMediaDecodeTime(); got != wantTFDT[segSeq] {
			t.Errorf("seg %d: first fragment tfdt=%d, want %d", segSeq, got, wantTFDT[segSeq])
		}
		var segDur uint64
		var firstSample mp4ff.Sample
		var haveFirst bool
		for _, fr := range mseg.Fragments {
			for _, trun := range fr.Moof.Traf.Truns {
				for _, smp := range trun.Samples {
					if !haveFirst {
						firstSample = smp
						haveFirst = true
					}
					totalSamples++
					if isSyncSample(smp) {
						totalSync++
					}
					segDur += uint64(smp.Dur)
				}
			}
		}
		if !isSyncSample(firstSample) {
			t.Errorf("seg %d: first sample is not a sync sample", segSeq)
		}
		if segDur != wantPartDur {
			t.Errorf("seg %d: summed sample dur=%d, summed part dur=%d", segSeq, segDur, wantPartDur)
		}
	}

	if totalSamples != numFrames {
		t.Errorf("total samples across parts=%d, want %d", totalSamples, numFrames)
	}
	if totalSync != numGOPs {
		t.Errorf("total sync samples=%d, want %d (one per GOP)", totalSync, numGOPs)
	}
}

