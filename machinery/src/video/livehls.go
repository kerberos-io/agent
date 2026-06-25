package video

import (
	"bytes"
	"fmt"

	mp4ff "github.com/Eyevinn/mp4ff/mp4"
	"github.com/kerberos-io/agent/machinery/src/log"
)

// LiveSegmenter turns a live stream of Annex B video samples into HLS-ready
// fragmented-MP4 (CMAF) output: ONE init segment (ftyp+moov) followed by a
// series of INDEPENDENT media segments (styp+moof+mdat), each beginning with a
// keyframe and carrying its own tfdt. This is the building block for the live
// HLS pipeline (agent -> hub-api -> vault -> hub-frontend) and is intentionally
// kept separate from the recording muxer in mp4.go:
//
//   - mp4.go writes ONE fragmented MP4 per recording (free-box placeholder up
//     front, back-filled on Close). That layout is great for archived files but
//     useless for live, where each segment must be shippable the instant it is
//     produced and must decode on its own after the init segment.
//   - LiveSegmenter emits discrete, self-contained segments via callbacks, so
//     the transport (single-POST to hub-api, drop-on-failure) never has to wait
//     for the recording to finish.
//
// Both producers use the SAME mp4ff fragment format, so live and archived video
// share one toolchain on the player side (hls.js #EXT-X-MAP + byte-range parts).
//
// The spike scope is video-only H.264/H.265. Audio and multi-track interleaving
// can be layered on later by adding tracks to the init segment and a second trun
// to each fragment; nothing here precludes that.
type LiveSegmenter struct {
	// codec is "H264"/"H265" (case handled in buildInit).
	codec string
	// timescale is the media timescale used in the init segment. The agent's
	// capture path feeds presentation timestamps in milliseconds, so a 1000-tick
	// timescale keeps sample durations exact with no rescaling.
	timescale uint32
	// targetSegmentMs is the minimum amount of media a segment accumulates before
	// the next keyframe is allowed to start a fresh segment. Keeping segments
	// keyframe-aligned is what makes each one independently decodable.
	targetSegmentMs uint64

	spsNALUs [][]byte
	ppsNALUs [][]byte
	vpsNALUs [][]byte

	// width/height are written into the visual sample entry. They are optional:
	// on a successful strict SPS parse mp4ff derives them, but the manual avcC
	// fallback (used for SPS that mp4ff cannot parse) needs them supplied.
	width  uint16
	height uint16

	videoTrackID uint32

	initSegment *mp4ff.InitSegment
	initBytes   []byte
	initEmitted bool

	seg   *mp4ff.MediaSegment
	frag  *mp4ff.Fragment
	seqNr uint32

	// started becomes true once the first segment has been opened.
	started bool
	// segStartPTS is the decode time (ms) of the first sample in the open
	// segment; elapsed media is measured against it to decide segment cuts.
	segStartPTS uint64
	// segDurationMs accumulates the committed sample durations of the open
	// segment so the playlist can advertise an accurate #EXTINF.
	segDurationMs uint64

	// pending holds the most recently received sample. Its duration is only known
	// once the NEXT sample arrives (duration = nextPTS - thisPTS), mirroring the
	// pending-sample pattern used by the recording muxer.
	pending *mp4ff.FullSample
	// lastDurationMs is the previous committed duration, reused to close out the
	// final pending sample (and to bridge non-monotonic timestamps).
	lastDurationMs uint64

	// OnInit is invoked exactly once with the encoded init segment bytes before
	// the first media segment is emitted. Optional.
	OnInit func(initBytes []byte) error
	// OnSegment is invoked once per completed media segment. Optional. It is left
	// unused in low-latency mode (see OnPart).
	OnSegment func(seg LiveSegment) error

	// --- Low-latency (LL-HLS) partial-segment mode ---
	//
	// When partTargetMs > 0 the segmenter additionally slices each segment into
	// ~partTargetMs CMAF "parts" (chunks) and emits them via OnPart the instant
	// each one closes, instead of waiting for the whole segment. The classic
	// per-segment OnSegment path above is left untouched (and unused) in this mode.
	// Each part is one mp4ff fragment (moof+mdat); part 0 of a segment also carries
	// the CMAF styp, so concatenating a segment's parts yields one valid segment.
	partTargetMs uint64
	// partFrag is the open part's fragment; partIndex is its 0-based index within
	// the current segment; fragSeq is the globally monotonic moof sequence number
	// shared across all parts (MSE wants increasing moof sequence numbers).
	partFrag        *mp4ff.Fragment
	partIndex       uint32
	fragSeq         uint32
	partSampleCount int
	partDurationMs  uint64
	partIndependent bool
	// OnPart is invoked once per completed CMAF part when partTargetMs > 0.
	OnPart func(part LivePart) error
}

// LiveSegment is one independently-decodable CMAF media segment.
type LiveSegment struct {
	// SequenceNumber is the monotonically increasing fragment sequence number
	// (also used as the moof sequence number and the seg-N.m4s index).
	SequenceNumber uint32
	// DurationMs is the summed sample duration of the segment, for #EXTINF.
	DurationMs uint64
	// Data is the complete styp+moof+mdat segment, ready to append after the init
	// segment and hand to hls.js / a vault object.
	Data []byte
}

// LivePart is one CMAF partial segment (chunk) of a media segment, emitted in
// low-latency mode the instant it closes - before the whole segment is done - so
// the playlist can advertise it via #EXT-X-PART for near-live playback.
type LivePart struct {
	// SegmentSeq is the parent media segment's sequence number (the N in
	// seg-N.K.m4s); PartIndex is K within that segment (0-based).
	SegmentSeq uint32
	PartIndex  uint32
	// Independent is true when the part begins with a keyframe (its first sample is
	// an IDR), i.e. it is independently decodable (#EXT-X-PART INDEPENDENT=YES).
	Independent bool
	// DurationMs is the summed sample duration of the part (for #EXT-X-PART).
	DurationMs uint64
	// Data of part 0 is styp+moof+mdat; later parts are bare moof+mdat, so
	// concatenating a segment's parts in order yields one valid CMAF segment.
	Data []byte
}

// Sample-entry flags matching the recording muxer so live and archived fragments
// describe random access points identically.
//
//	keyframe     0x02000000 = sampleDependsOn=2 (depends on nothing), sync sample
//	non-keyframe 0x01010000 = sampleDependsOn=1, sampleIsNonSyncSample=1
const (
	liveSyncSampleFlags    uint32 = 0x02000000
	liveNonSyncSampleFlags uint32 = 0x01010000
	// liveFallbackDurationMs is used when a duration cannot be derived (first
	// frame at Close, or non-monotonic timestamps) and no prior duration exists.
	// ~33 ms approximates 30 fps and is only ever a single-frame nicety.
	liveFallbackDurationMs uint64 = 33
)

// NewLiveSegmenter creates a video-only live segmenter for the given codec.
// spsNALUs/ppsNALUs (and vpsNALUs for H.265) may be raw NAL units or Annex B
// blobs with start codes; both are normalized. targetSegmentMs is clamped to a
// sane floor so a misconfiguration cannot produce one-frame segments.
func NewLiveSegmenter(codec string, spsNALUs, ppsNALUs, vpsNALUs [][]byte, targetSegmentMs uint64) *LiveSegmenter {
	if targetSegmentMs < 500 {
		targetSegmentMs = 500
	}
	return &LiveSegmenter{
		codec:           codec,
		timescale:       1000,
		targetSegmentMs: targetSegmentMs,
		spsNALUs:        spsNALUs,
		ppsNALUs:        ppsNALUs,
		vpsNALUs:        vpsNALUs,
	}
}

// SetDimensions records the encoded video width/height in pixels. They are
// written into the avc1/hvc1 visual sample entry and are required for the manual
// descriptor fallback path (SPS that mp4ff's strict parser rejects).
func (ls *LiveSegmenter) SetDimensions(width, height uint16) {
	ls.width = width
	ls.height = height
}

// EnableLowLatency switches the segmenter into LL-HLS mode, additionally slicing
// each segment into ~partTargetMs CMAF parts emitted via OnPart as they close.
// partTargetMs is clamped to a sane floor. Call before the first WriteSample.
func (ls *LiveSegmenter) EnableLowLatency(partTargetMs uint64) {
	if partTargetMs < 100 {
		partTargetMs = 100
	}
	ls.partTargetMs = partTargetMs
}

// InitSegment returns the encoded init segment bytes, building them on demand.
// Useful for tests and for serving the #EXT-X-MAP target without waiting for the
// first media segment.
func (ls *LiveSegmenter) InitSegment() ([]byte, error) {
	if ls.initBytes == nil {
		if err := ls.buildInit(); err != nil {
			return nil, err
		}
	}
	return ls.initBytes, nil
}

// buildInit constructs the ftyp+moov init segment from the parameter sets.
func (ls *LiveSegmenter) buildInit() error {
	init := mp4ff.CreateEmptyInit()
	init.AddEmptyTrack(ls.timescale, "video", "und")
	trak := init.Moov.Traks[0]

	switch ls.codec {
	case "H264", "h264", "AVC", "avc", "AVC1", "avc1":
		sps, pps := normalizeH264ParameterSets(ls.spsNALUs, ls.ppsNALUs)
		if len(sps) == 0 || len(pps) == 0 {
			return fmt.Errorf("livehls: missing H264 SPS/PPS (sps=%d pps=%d)", len(sps), len(pps))
		}
		// includePS=true stores SPS/PPS in the avcC so segments need not carry
		// in-band parameter sets - browsers read them from the init segment. Some
		// camera SPS variants trip mp4ff's strict parser (e.g. unusual VUI/SAR);
		// fall back to a manually built avcC just like the recording muxer does so
		// those cameras still produce a valid init segment.
		if err := trak.SetAVCDescriptor("avc1", sps, pps, true); err != nil {
			log.Log.Warning("livehls: SetAVCDescriptor failed, using manual avcC fallback: " + err.Error())
			if fbErr := addAVCDescriptorFallback(trak, sps, pps, ls.width, ls.height); fbErr != nil {
				return fmt.Errorf("livehls: AVC descriptor fallback: %w", fbErr)
			}
		}
	case "H265", "h265", "HEVC", "hevc", "HVC1", "hvc1":
		vps, sps, pps := normalizeH265ParameterSets(ls.vpsNALUs, ls.spsNALUs, ls.ppsNALUs)
		if len(vps) == 0 || len(sps) == 0 || len(pps) == 0 {
			return fmt.Errorf("livehls: missing H265 VPS/SPS/PPS (vps=%d sps=%d pps=%d)", len(vps), len(sps), len(pps))
		}
		if err := trak.SetHEVCDescriptor("hvc1", vps, sps, pps, [][]byte{}, true); err != nil {
			return fmt.Errorf("livehls: SetHEVCDescriptor: %w", err)
		}
	default:
		return fmt.Errorf("livehls: unsupported codec %q", ls.codec)
	}

	// Record the encoded dimensions in the track header when known.
	if ls.width > 0 && ls.height > 0 {
		trak.Tkhd.Width = mp4ff.Fixed32(uint32(ls.width) << 16)
		trak.Tkhd.Height = mp4ff.Fixed32(uint32(ls.height) << 16)
	}
	// mdhd.Duration MUST be 0 for fragmented MP4 so players derive duration from
	// the fragments rather than a (here unknown) total.
	trak.Mdia.Mdhd.Duration = 0

	ls.videoTrackID = trak.Tkhd.TrackID

	var buf bytes.Buffer
	if err := init.Encode(&buf); err != nil {
		return fmt.Errorf("livehls: encode init: %w", err)
	}
	ls.initSegment = init
	ls.initBytes = buf.Bytes()
	return nil
}

// WriteSample feeds one Annex B access unit with its decode timestamp (DTS) in
// milliseconds. The first sample of a session MUST be a keyframe; a non-keyframe
// first sample is dropped (it could not be decoded without a preceding IDR).
//
// compositionOffsetMs is the CTS offset (PTS-DTS, for B-frame reordering) in
// timescale ticks; pass 0 for streams without B-frames.
func (ls *LiveSegmenter) WriteSample(isKeyframe bool, annexB []byte, ptsMs uint64, compositionOffsetMs int32) error {
	// Lazily build + emit the init segment on the first accepted sample.
	if ls.initBytes == nil {
		if err := ls.buildInit(); err != nil {
			return err
		}
	}
	if !ls.initEmitted {
		ls.initEmitted = true
		if ls.OnInit != nil {
			if err := ls.OnInit(ls.initBytes); err != nil {
				return err
			}
		}
	}

	// A session must open on a random-access point; otherwise the first segment
	// would reference frames that never arrived.
	if !ls.started && !isKeyframe {
		log.Log.Debug("LiveSegmenter.WriteSample(): dropping leading non-keyframe before first IDR")
		return nil
	}

	lengthPrefixed, err := annexBToLengthPrefixed(annexB)
	if err != nil {
		return fmt.Errorf("livehls: convert AnnexB: %w", err)
	}

	// Low-latency mode slices each segment into parts; the classic per-segment path
	// below is left exactly as-is for the default (non-LL) configuration.
	if ls.partTargetMs > 0 {
		return ls.writeSampleLL(isKeyframe, lengthPrefixed, ptsMs, compositionOffsetMs)
	}

	// The previous sample's duration is the gap to this sample's PTS. Commit it
	// to the (still open) current fragment before we consider rolling segments,
	// because the pending sample always precedes this one in decode order.
	if ls.pending != nil {
		dur := ls.lastDurationMs
		if ptsMs > ls.pending.DecodeTime {
			dur = ptsMs - ls.pending.DecodeTime
		}
		if dur == 0 {
			dur = liveFallbackDurationMs
		}
		ls.lastDurationMs = dur
		ls.pending.Sample.Dur = uint32(dur)
		if err := ls.commitPending(); err != nil {
			return err
		}
	}

	// At every keyframe, decide whether enough media has accumulated to close the
	// open segment and start a new one. Cutting only on keyframes guarantees each
	// segment is independently decodable.
	if isKeyframe {
		shouldCut := !ls.started || (ptsMs-ls.segStartPTS) >= ls.targetSegmentMs
		if shouldCut {
			if ls.started {
				if err := ls.emitSegment(); err != nil {
					return err
				}
			}
			ls.openSegment(ptsMs)
		}
	}

	// Stage this sample; its duration is filled in when the next sample arrives
	// (or at Close()).
	flags := liveNonSyncSampleFlags
	if isKeyframe {
		flags = liveSyncSampleFlags
	}
	ls.pending = &mp4ff.FullSample{
		Sample: mp4ff.Sample{
			Flags:                 flags,
			Size:                  uint32(len(lengthPrefixed)),
			CompositionTimeOffset: compositionOffsetMs,
		},
		DecodeTime: ptsMs,
		Data:       lengthPrefixed,
	}
	return nil
}

// openSegment starts a fresh media segment (with CMAF styp) and an empty
// single-track fragment whose moof sequence number is the segment index.
func (ls *LiveSegmenter) openSegment(startPTS uint64) {
	ls.seqNr++
	ls.seg = mp4ff.NewMediaSegment() // includes a CMAF styp box by default
	frag, err := mp4ff.CreateFragment(ls.seqNr, ls.videoTrackID)
	if err != nil {
		log.Log.Error("LiveSegmenter.openSegment(): CreateFragment failed: " + err.Error())
		return
	}
	ls.seg.AddFragment(frag)
	ls.frag = frag
	ls.segStartPTS = startPTS
	ls.segDurationMs = 0
	ls.started = true
}

// commitPending appends the staged sample to the open fragment. The first sample
// of a fragment seeds the tfdt baseMediaDecodeTime from its absolute DecodeTime,
// which is what makes the segment independently seekable/decodable.
func (ls *LiveSegmenter) commitPending() error {
	if ls.pending == nil {
		return nil
	}
	if ls.frag == nil {
		// No open segment yet (e.g. pending set before the first keyframe cut). The
		// keyframe path always opens a segment before staging, so this only guards
		// against logic drift; drop rather than panic.
		ls.pending = nil
		return nil
	}
	if err := ls.frag.AddFullSampleToTrack(*ls.pending, ls.videoTrackID); err != nil {
		return fmt.Errorf("livehls: AddFullSampleToTrack: %w", err)
	}
	ls.segDurationMs += uint64(ls.pending.Sample.Dur)
	ls.pending = nil
	return nil
}

// emitSegment encodes the open segment and hands it to OnSegment.
func (ls *LiveSegmenter) emitSegment() error {
	if ls.seg == nil {
		return nil
	}
	var buf bytes.Buffer
	if err := ls.seg.Encode(&buf); err != nil {
		return fmt.Errorf("livehls: encode segment %d: %w", ls.seqNr, err)
	}
	out := LiveSegment{
		SequenceNumber: ls.seqNr,
		DurationMs:     ls.segDurationMs,
		Data:           buf.Bytes(),
	}
	ls.seg = nil
	ls.frag = nil
	if ls.OnSegment != nil {
		return ls.OnSegment(out)
	}
	return nil
}

// Close flushes the final pending sample and emits the last open segment (or, in
// low-latency mode, the last open part). Call once when the live session ends so
// no trailing media is lost.
func (ls *LiveSegmenter) Close() error {
	if ls.partTargetMs > 0 {
		if ls.pending != nil {
			dur := ls.lastDurationMs
			if dur == 0 {
				dur = liveFallbackDurationMs
			}
			ls.pending.Sample.Dur = uint32(dur)
			if err := ls.commitPendingPart(); err != nil {
				return err
			}
		}
		return ls.closePart()
	}
	if ls.pending != nil {
		dur := ls.lastDurationMs
		if dur == 0 {
			dur = liveFallbackDurationMs
		}
		ls.pending.Sample.Dur = uint32(dur)
		if err := ls.commitPending(); err != nil {
			return err
		}
	}
	return ls.emitSegment()
}

// writeSampleLL is the low-latency counterpart of the per-segment staging in
// WriteSample: it commits the previous sample into the open part, rolls the part
// (every ~partTargetMs) and the segment (at keyframes, every ~targetSegmentMs),
// then stages the current sample. Parts are emitted via OnPart as they close.
func (ls *LiveSegmenter) writeSampleLL(isKeyframe bool, lengthPrefixed []byte, ptsMs uint64, compositionOffsetMs int32) error {
	if ls.pending != nil {
		dur := ls.lastDurationMs
		if ptsMs > ls.pending.DecodeTime {
			dur = ptsMs - ls.pending.DecodeTime
		}
		if dur == 0 {
			dur = liveFallbackDurationMs
		}
		ls.lastDurationMs = dur
		ls.pending.Sample.Dur = uint32(dur)
		if err := ls.commitPendingPart(); err != nil {
			return err
		}
	}

	// Roll the segment at keyframes once enough media accumulated; otherwise roll a
	// part once it reaches the part target. The two are mutually exclusive: a
	// keyframe cut also closes the current part.
	cut := false
	if isKeyframe {
		cut = !ls.started || (ptsMs-ls.segStartPTS) >= ls.targetSegmentMs
	}
	switch {
	case cut:
		if ls.started {
			if err := ls.closePart(); err != nil {
				return err
			}
		}
		ls.openSegmentLL(ptsMs)
	case ls.started && ls.partDurationMs >= ls.partTargetMs:
		if err := ls.closePart(); err != nil {
			return err
		}
		ls.openPartLL()
	}

	flags := liveNonSyncSampleFlags
	if isKeyframe {
		flags = liveSyncSampleFlags
	}
	ls.pending = &mp4ff.FullSample{
		Sample: mp4ff.Sample{
			Flags:                 flags,
			Size:                  uint32(len(lengthPrefixed)),
			CompositionTimeOffset: compositionOffsetMs,
		},
		DecodeTime: ptsMs,
		Data:       lengthPrefixed,
	}
	return nil
}

// commitPendingPart appends the staged sample to the open part fragment, marking
// the part independent when its first sample is a keyframe.
func (ls *LiveSegmenter) commitPendingPart() error {
	if ls.pending == nil {
		return nil
	}
	if ls.partFrag == nil {
		// No open part yet (pending staged before the first keyframe cut). The cut
		// path always opens a part before staging, so this only guards against logic
		// drift; drop rather than panic.
		ls.pending = nil
		return nil
	}
	first := ls.partSampleCount == 0
	if err := ls.partFrag.AddFullSampleToTrack(*ls.pending, ls.videoTrackID); err != nil {
		return fmt.Errorf("livehls: AddFullSampleToTrack: %w", err)
	}
	if first && ls.pending.Sample.Flags == liveSyncSampleFlags {
		ls.partIndependent = true
	}
	ls.partSampleCount++
	ls.partDurationMs += uint64(ls.pending.Sample.Dur)
	ls.segDurationMs += uint64(ls.pending.Sample.Dur)
	ls.pending = nil
	return nil
}

// openSegmentLL starts a fresh media segment at a keyframe by opening its part 0.
func (ls *LiveSegmenter) openSegmentLL(startPTS uint64) {
	ls.seqNr++
	ls.partIndex = 0
	ls.segStartPTS = startPTS
	ls.segDurationMs = 0
	ls.started = true
	ls.openPartFragment()
}

// openPartLL starts the next part within the current segment.
func (ls *LiveSegmenter) openPartLL() {
	ls.partIndex++
	ls.openPartFragment()
}

// openPartFragment allocates a fresh single-track fragment (one moof+mdat) for
// the next part, with a globally monotonic moof sequence number.
func (ls *LiveSegmenter) openPartFragment() {
	ls.fragSeq++
	frag, err := mp4ff.CreateFragment(ls.fragSeq, ls.videoTrackID)
	if err != nil {
		log.Log.Error("LiveSegmenter.openPartFragment(): CreateFragment failed: " + err.Error())
		return
	}
	ls.partFrag = frag
	ls.partSampleCount = 0
	ls.partDurationMs = 0
	ls.partIndependent = false
}

// closePart encodes the open part and hands it to OnPart. Part 0 of a segment
// carries the CMAF styp; later parts are bare moof+mdat, so a segment's parts
// concatenate into one valid segment. Empty parts are skipped.
func (ls *LiveSegmenter) closePart() error {
	if ls.partFrag == nil || ls.partSampleCount == 0 {
		return nil
	}
	var buf bytes.Buffer
	if ls.partIndex == 0 {
		seg := mp4ff.NewMediaSegment() // includes a CMAF styp box by default
		seg.AddFragment(ls.partFrag)
		if err := seg.Encode(&buf); err != nil {
			return fmt.Errorf("livehls: encode part %d.%d: %w", ls.seqNr, ls.partIndex, err)
		}
	} else {
		if err := ls.partFrag.Encode(&buf); err != nil {
			return fmt.Errorf("livehls: encode part %d.%d: %w", ls.seqNr, ls.partIndex, err)
		}
	}
	out := LivePart{
		SegmentSeq:  ls.seqNr,
		PartIndex:   ls.partIndex,
		Independent: ls.partIndependent,
		DurationMs:  ls.partDurationMs,
		Data:        buf.Bytes(),
	}
	ls.partFrag = nil
	if ls.OnPart != nil {
		return ls.OnPart(out)
	}
	return nil
}
