package video

import (
	"bufio"
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	mp4ff "github.com/Eyevinn/mp4ff/mp4"
	"github.com/kerberos-io/agent/machinery/src/encryption"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/utils"
)

var LastPTS uint64 = 0 // Last PTS for the current segment

// MacEpochOffset is the number of seconds between Mac HFS epoch (1904-01-01)
// and Unix epoch (1970-01-01). QuickTime requires timestamps in Mac HFS format.
const MacEpochOffset uint64 = 2082844800

// FragmentDurationMs is the target duration for each fragment in milliseconds.
// Fragments will be flushed at the first keyframe after this duration has elapsed,
// resulting in ~3 second fragments (assuming a typical GOP interval).
const FragmentDurationMs = 3000

type MP4 struct {
	// FileName is the name of the file
	FileName                string
	width                   int
	height                  int
	Segments                []*mp4ff.MediaSegment // List of media segments
	Segment                 *mp4ff.MediaSegment
	MultiTrackFragment      *mp4ff.Fragment
	TrackIDs                []uint32
	FileWriter              *os.File
	Writer                  *bufio.Writer
	SegmentCount            int
	SampleCount             int
	StartPTS                uint64
	VideoTotalDuration      uint64
	AudioTotalDuration      uint64
	AudioPTS                uint64
	Start                   bool
	SPSNALUs                [][]byte // SPS NALUs for H264
	PPSNALUs                [][]byte // PPS NALUs for H264
	VPSNALUs                [][]byte // VPS NALUs for H264
	FreeBoxSize             int64
	FragmentStartRawPTS     uint64            // Raw PTS for timing when to flush fragments
	FragmentStartDTS        uint64            // Accumulated VideoTotalDuration at fragment start (matches tfdt)
	MoofBoxes               int64             // Number of moof boxes in the file
	MoofBoxSizes            []int64           // Sizes of each moof box
	SegmentDurations        []uint64          // Duration of each segment in timescale units
	SegmentBaseDecTimes     []uint64          // Base decode time of each segment
	StartTime               uint64            // Start time of the MP4 file
	VideoTrackName          string            // Name of the video track
	VideoTrack              int               // Track ID for the video track
	AudioTrackName          string            // Name of the audio track
	AudioTrack              int               // Track ID for the audio track
	VideoFullSample         *mp4ff.FullSample // Full sample for video track
	AudioFullSample         *mp4ff.FullSample // Full sample for audio track
	LastAudioSampleDTS      uint64            // Last PTS for audio sample
	LastVideoSampleDTS      uint64            // Last PTS for video sample
	SampleType              string            // Type of the sample (e.g., "video", "audio", "subtitle")
	TotalKeyframesReceived  int               // Total keyframes received by AddSampleToTrack
	TotalKeyframesWritten   int               // Total keyframes written to trun boxes
	FragmentKeyframeCount   int               // Keyframes in the current fragment
	PendingSampleIsKeyframe bool              // Whether the pending video sample is a keyframe
}

// NewMP4 creates a new MP4 object.
// maxDurationSec is the maximum expected recording duration in seconds,
// used to calculate the free-box placeholder size for ftyp+moov+sidx.
func NewMP4(fileName string, spsNALUs [][]byte, ppsNALUs [][]byte, vpsNALUs [][]byte, maxDurationSec int64) *MP4 {

	// Calculate the placeholder size needed at the start of the file.
	// Components:
	//   ftyp:  ~32 bytes
	//   moov:  ~1500 bytes (mvhd + mvex + video trak + audio trak + UUID)
	//   sidx:  24 bytes fixed + 12 bytes per segment reference
	// Segments are ~FragmentDurationMs each, so:
	//   numSegments = ceil(maxDurationSec * 1000 / FragmentDurationMs) + 1 (safety margin)
	//   sidxSize    = 24 + 12 * numSegments
	baseSize := int64(2560) // ftyp + moov + extra headroom for large UUID signatures
	numSegments := int64(0)
	if maxDurationSec > 0 {
		// Use integer ceiling division to avoid underestimating the number of segments.
		numSegments = ((maxDurationSec*1000)+FragmentDurationMs-1)/FragmentDurationMs + 1
	}
	sidxSize := int64(24 + 12*numSegments)
	freeBoxSize := int(baseSize + sidxSize)
	free := mp4ff.NewFreeBox(make([]byte, freeBoxSize))

	// Create a writer
	ofd, err := os.Create(fileName)
	if err != nil {
	}

	// Create a buffered writer
	bufferedWriter := bufio.NewWriterSize(ofd, 64*1024) // 64KB buffer

	// Write the free box placeholder at the start of the file
	err = free.Encode(bufferedWriter)
	if err != nil {
	}

	return &MP4{
		FileName:    fileName,
		StartTime:   uint64(time.Now().Unix()),
		FreeBoxSize: int64(freeBoxSize) + 8, // payload + 8 byte box header
		FileWriter:  ofd,
		Writer:      bufferedWriter,
		SPSNALUs:    spsNALUs,
		PPSNALUs:    ppsNALUs,
		VPSNALUs:    vpsNALUs,
	}
}

// SetWidth sets the width of the video
func (mp4 *MP4) SetWidth(width int) {
	// Set the width of the video
	mp4.width = width
}

// SetHeight sets the height of the video
func (mp4 *MP4) SetHeight(height int) {
	// Set the height of the video
	mp4.height = height
}

// AddVideoTrack
// Add a video track to the MP4 file
func (mp4 *MP4) AddVideoTrack(codec string) uint32 {
	nextTrack := uint32(len(mp4.TrackIDs) + 1)
	mp4.VideoTrack = int(nextTrack)
	mp4.TrackIDs = append(mp4.TrackIDs, nextTrack)
	mp4.VideoTrackName = codec
	return nextTrack
}

// AddAudioTrack
// Add an audio track to the MP4 file
func (mp4 *MP4) AddAudioTrack(codec string) uint32 {
	nextTrack := uint32(len(mp4.TrackIDs) + 1)
	mp4.AudioTrack = int(nextTrack)
	mp4.TrackIDs = append(mp4.TrackIDs, nextTrack)
	mp4.AudioTrackName = codec
	return nextTrack
}

func (mp4 *MP4) AddMediaSegment(segNr int) {
}

// flushPendingVideoSample writes the pending video sample to the current fragment.
// If nextPTS is provided (non-zero), it calculates duration from the PTS difference.
// If nextPTS is 0 (e.g., at Close time), it uses the last known duration.
// Returns true if a sample was flushed, false if there was no pending sample.
func (mp4 *MP4) flushPendingVideoSample(nextPTS uint64) bool {
	if mp4.VideoFullSample == nil || mp4.MultiTrackFragment == nil {
		return false
	}

	var duration uint64
	if nextPTS > 0 && nextPTS > mp4.VideoFullSample.DecodeTime {
		duration = nextPTS - mp4.VideoFullSample.DecodeTime
	} else {
		// No valid nextPTS (Close case) or PTS went backwards (jitter/discontinuity)
		if nextPTS > 0 {
			log.Log.Warning(fmt.Sprintf("mp4.flushPendingVideoSample(): video PTS went backwards or zero duration (nextPTS=%d, prevDTS=%d), using last known duration", nextPTS, mp4.VideoFullSample.DecodeTime))
		}
		duration = mp4.LastVideoSampleDTS
		if duration == 0 {
			duration = 33 // Default ~30fps frame duration
		}
	}

	mp4.LastVideoSampleDTS = duration
	mp4.VideoTotalDuration += duration
	mp4.VideoFullSample.DecodeTime = mp4.VideoTotalDuration - duration
	mp4.VideoFullSample.Sample.Dur = uint32(duration)

	isKF := mp4.PendingSampleIsKeyframe
	err := mp4.MultiTrackFragment.AddFullSampleToTrack(*mp4.VideoFullSample, uint32(mp4.VideoTrack))
	if err != nil {
		log.Log.Error("mp4.flushPendingVideoSample(): error adding sample: " + err.Error())
	}
	if isKF {
		mp4.TotalKeyframesWritten++
		mp4.FragmentKeyframeCount++
		log.Log.Debug(fmt.Sprintf("mp4.flushPendingVideoSample(): KEYFRAME WRITTEN to trun - totalWritten=%d, fragmentKF=%d, flags=0x%08x, dur=%d, DTS=%d",
			mp4.TotalKeyframesWritten, mp4.FragmentKeyframeCount, mp4.VideoFullSample.Sample.Flags, duration, mp4.VideoFullSample.DecodeTime))
	}

	mp4.VideoFullSample = nil
	mp4.PendingSampleIsKeyframe = false
	return true
}

func (mp4 *MP4) AddSampleToTrack(trackID uint32, isKeyframe bool, data []byte, pts uint64) error {

	if isKeyframe && trackID == uint32(mp4.VideoTrack) {
		mp4.TotalKeyframesReceived++
		elapsedDbg := uint64(0)
		if mp4.Start {
			elapsedDbg = pts - mp4.FragmentStartRawPTS
		}
		log.Log.Debug(fmt.Sprintf("mp4.AddSampleToTrack(): KEYFRAME #%d received - PTS=%d, size=%d, elapsed=%dms, started=%t, segment=%d, fragKF=%d",
			mp4.TotalKeyframesReceived, pts, len(data), elapsedDbg, mp4.Start, mp4.SegmentCount, mp4.FragmentKeyframeCount))
	}

	if isKeyframe {

		// Determine whether to start a new fragment.
		// We only flush at a keyframe boundary once at least FragmentDurationMs
		// of content has been accumulated, resulting in ~3 second fragments.
		elapsed := uint64(0)
		if mp4.Start {
			elapsed = pts - mp4.FragmentStartRawPTS
		}
		shouldFlush := !mp4.Start || elapsed >= FragmentDurationMs

		if shouldFlush {
			// Write the previous segment to the file
			if mp4.Start {
				// IMPORTANT: Add any pending video sample to the current segment BEFORE flushing.
				// This ensures the segment contains all frames up to (but not including) this keyframe,
				// and the new segment will start cleanly with this keyframe.
				if trackID == uint32(mp4.VideoTrack) {
					mp4.flushPendingVideoSample(pts)
				}

				log.Log.Debug(fmt.Sprintf("mp4.AddSampleToTrack(): FLUSHING segment #%d - keyframes_in_fragment=%d, totalKF_received=%d, totalKF_written=%d",
					mp4.SegmentCount, mp4.FragmentKeyframeCount, mp4.TotalKeyframesReceived, mp4.TotalKeyframesWritten))
				mp4.MoofBoxes = mp4.MoofBoxes + 1
				mp4.MoofBoxSizes = append(mp4.MoofBoxSizes, int64(mp4.Segment.Size()))
				// Track the segment's duration and base decode time for sidx.
				// Use accumulated VideoTotalDuration which matches the tfdt values
				// in the trun boxes, NOT raw PTS from the camera.
				segDuration := mp4.VideoTotalDuration - mp4.FragmentStartDTS
				mp4.SegmentDurations = append(mp4.SegmentDurations, segDuration)
				mp4.SegmentBaseDecTimes = append(mp4.SegmentBaseDecTimes, mp4.FragmentStartDTS)
				err := mp4.Segment.Encode(mp4.Writer)
				if err != nil {
					log.Log.Error("mp4.AddSampleToTrack(): error encoding segment: " + err.Error())
				}
				mp4.Segments = append(mp4.Segments, mp4.Segment)
			}

			mp4.Start = true

			// Increment the segment count
			mp4.SegmentCount = mp4.SegmentCount + 1

			// Create a new media segment
			seg := mp4ff.NewMediaSegment()

			// Create a video fragment
			multiTrackFragment, err := mp4ff.CreateMultiTrackFragment(uint32(mp4.SegmentCount), mp4.TrackIDs)
			if err != nil {
				log.Log.Error("mp4.AddSampleToTrack(): error creating multi track fragment: " + err.Error())
			}
			mp4.MultiTrackFragment = multiTrackFragment
			seg.AddFragment(multiTrackFragment)

			// Set to MP4 struct
			mp4.Segment = seg

			// Set the start PTS for the next segment
			mp4.StartPTS = pts
			mp4.FragmentStartRawPTS = pts
			mp4.FragmentStartDTS = mp4.VideoTotalDuration
			mp4.FragmentKeyframeCount = 0 // Reset keyframe counter for new fragment
		}
	}

	if mp4.Start {

		if trackID == uint32(mp4.VideoTrack) {

			var lengthPrefixed []byte
			var err error
			switch mp4.VideoTrackName {
			case "H264", "AVC1": // Convert Annex B to length-prefixed NAL units if H264
				lengthPrefixed, err = annexBToLengthPrefixed(data)
			case "H265", "HVC1": // Convert H265 Annex B to length-prefixed NAL units
				lengthPrefixed, err = annexBToLengthPrefixed(data)
			}

			if err == nil {
				// Flush previous pending sample before storing the new one
				if mp4.VideoFullSample != nil {
					log.Log.Debug("Adding sample to track " + fmt.Sprintf("%d, PTS: %d, size: %d, Keyframe: %t", trackID, pts, len(lengthPrefixed), isKeyframe))
					mp4.flushPendingVideoSample(pts)
				}

				// Set the sample data
				var fullSample mp4ff.FullSample
				flags := uint32(33554432)
				if !isKeyframe {
					flags = uint32(16842752)
				}
				fullSample.DecodeTime = pts
				fullSample.Data = lengthPrefixed
				fullSample.Sample = mp4ff.Sample{
					Size:                  uint32(len(fullSample.Data)),
					Flags:                 flags,
					CompositionTimeOffset: 0, // No composition time offset for video
				}
				mp4.VideoFullSample = &fullSample
				mp4.PendingSampleIsKeyframe = isKeyframe
				mp4.SampleType = "video"
			}
		} else if trackID == uint32(mp4.AudioTrack) {
			if mp4.AudioFullSample != nil {
				SplitAACFrame(mp4.AudioFullSample.Data, func(started bool, aac []byte) {
					sampleToAdd := *mp4.AudioFullSample
					dts := pts - mp4.AudioFullSample.DecodeTime
					if pts < mp4.AudioFullSample.DecodeTime {
						//log.Printf("Warning: PTS %d is less than previous sample's DecodeTime %d, resetting AudioFullSample", pts, mp4.AudioFullSample.DecodeTime)
						dts = 1
					}
					if started {
						dts = 1
					}
					mp4.LastAudioSampleDTS = dts
					//fmt.Printf("Adding sample to track %d, PTS: %d, Duration: %d, size: %d\n", trackID, pts, dts, len(aac[7:]))
					mp4.AudioTotalDuration += dts
					mp4.AudioPTS += dts
					sampleToAdd.Data = aac[7:] // Remove the ADTS header (first 7 bytes)
					sampleToAdd.DecodeTime = mp4.AudioPTS - dts
					sampleToAdd.Sample.Dur = uint32(dts)
					sampleToAdd.Sample.Size = uint32(len(aac[7:]))
					err := mp4.MultiTrackFragment.AddFullSampleToTrack(sampleToAdd, trackID)
					if err != nil {
						log.Log.Error("mp4.AddSampleToTrack(): error adding sample to track " + fmt.Sprintf("%d: %v", trackID, err))
					}
				})
			}

			// Set the sample data
			//flags := uint32(33554432)
			var fullSample mp4ff.FullSample
			fullSample.DecodeTime = pts
			fullSample.Data = data
			fullSample.Sample = mp4ff.Sample{
				Size:                  uint32(len(fullSample.Data)),
				Flags:                 0,
				CompositionTimeOffset: 0, // No composition time offset for audio
			}
			mp4.AudioFullSample = &fullSample
			mp4.SampleType = "audio"
		}
	}

	return nil
}

func (mp4 *MP4) Close(config *models.Config) {

	log.Log.Info(fmt.Sprintf("mp4.Close(): KEYFRAME SUMMARY - totalReceived=%d, totalWritten=%d, segments=%d, lastFragmentKF=%d",
		mp4.TotalKeyframesReceived, mp4.TotalKeyframesWritten, mp4.SegmentCount, mp4.FragmentKeyframeCount))

	if mp4.VideoTotalDuration == 0 && mp4.AudioTotalDuration == 0 {
		log.Log.Error("mp4.Close(): no video or audio samples added, cannot create MP4 file")
	}

	// Add final pending samples before closing
	if mp4.Segment != nil {
		// Add final video sample if pending (pass 0 as nextPTS to use last known duration)
		mp4.flushPendingVideoSample(0)

		// Add final audio sample if pending
		if mp4.AudioFullSample != nil && mp4.AudioTrack > 0 {
			SplitAACFrame(mp4.AudioFullSample.Data, func(started bool, aac []byte) {
				sampleToAdd := *mp4.AudioFullSample
				dts := mp4.LastAudioSampleDTS
				if dts == 0 {
					dts = 1024 // Default AAC frame duration
				}
				mp4.AudioTotalDuration += dts
				mp4.AudioPTS += dts
				sampleToAdd.Data = aac[7:]
				sampleToAdd.DecodeTime = mp4.AudioPTS - dts
				sampleToAdd.Sample.Dur = uint32(dts)
				sampleToAdd.Sample.Size = uint32(len(aac[7:]))
				err := mp4.MultiTrackFragment.AddFullSampleToTrack(sampleToAdd, uint32(mp4.AudioTrack))
				if err != nil {
					log.Log.Error("mp4.Close(): error adding final audio sample: " + err.Error())
				}
			})
			mp4.AudioFullSample = nil
		}
	}

	// Encode the last segment
	if mp4.Segment != nil {
		// Track the last segment's size, duration and base decode time.
		// Use accumulated VideoTotalDuration which matches tfdt values.
		mp4.MoofBoxes = mp4.MoofBoxes + 1
		mp4.MoofBoxSizes = append(mp4.MoofBoxSizes, int64(mp4.Segment.Size()))
		lastSegDuration := mp4.VideoTotalDuration - mp4.FragmentStartDTS
		if lastSegDuration == 0 {
			lastSegDuration = mp4.LastVideoSampleDTS
		}
		mp4.SegmentDurations = append(mp4.SegmentDurations, lastSegDuration)
		mp4.SegmentBaseDecTimes = append(mp4.SegmentBaseDecTimes, mp4.FragmentStartDTS)

		err := mp4.Segment.Encode(mp4.Writer)
		if err != nil {
			log.Log.Error("mp4.Close(): error encoding last segment: " + err.Error())
		}
	}

	mp4.Writer.Flush()
	// Ensure all segment data is on disk before we overwrite the placeholder at offset 0.
	if err := mp4.FileWriter.Sync(); err != nil {
		log.Log.Error("mp4.Close(): error syncing file: " + err.Error())
	}

	// Now we have all the moof and mdat boxes written to the file.
	// We build the ftyp + moov init segment and write it at the start,
	// overwriting the free box placeholder we reserved in NewMP4.
	init := mp4ff.NewMP4Init()

	// Create a new ftyp box
	majorBrand := "isom"
	minorVersion := uint32(512)
	compatibleBrands := []string{"iso2", "avc1", "hvc1", "mp41"}
	ftyp := mp4ff.NewFtyp(majorBrand, minorVersion, compatibleBrands)
	init.AddChild(ftyp)

	// Create a new moov box
	moov := mp4ff.NewMoovBox()
	init.AddChild(moov)

	// Set the creation time and modification time for the moov box.
	// QuickTime requires timestamps in Mac HFS format (seconds since 1904-01-01),
	// so we convert from Unix epoch by adding MacEpochOffset.
	videoTimescale := uint32(1000)
	audioTimescale := uint32(1000)
	macTime := mp4.StartTime + MacEpochOffset
	nextTrackID := uint32(len(mp4.TrackIDs) + 1)
	mvhd := &mp4ff.MvhdBox{
		Version:          0,
		Flags:            0,
		CreationTime:     macTime,
		ModificationTime: macTime,
		Timescale:        videoTimescale,
		Duration:         mp4.VideoTotalDuration,
		Rate:             0x00010000, // 1.0 playback speed (16.16 fixed point)
		Volume:           0x0100,     // 1.0 full volume (8.8 fixed point)
		NextTrackID:      nextTrackID,
	}
	init.Moov.AddChild(mvhd)

	// Set the total duration in the moov box
	mvex := mp4ff.NewMvexBox()
	mvex.AddChild(&mp4ff.MehdBox{FragmentDuration: int64(mp4.VideoTotalDuration)})
	init.Moov.AddChild(mvex)

	// Add a track for the video
	switch mp4.VideoTrackName {
	case "H264", "AVC1":
		init.AddEmptyTrack(videoTimescale, "video", "und")
		includePS := true
		err := init.Moov.Traks[0].SetAVCDescriptor("avc1", mp4.SPSNALUs, mp4.PPSNALUs, includePS)
		if err != nil {
		}
		init.Moov.Traks[0].Tkhd.Duration = mp4.VideoTotalDuration
		init.Moov.Traks[0].Tkhd.Width = mp4ff.Fixed32(uint32(mp4.width) << 16)
		init.Moov.Traks[0].Tkhd.Height = mp4ff.Fixed32(uint32(mp4.height) << 16)
		init.Moov.Traks[0].Tkhd.CreationTime = macTime
		init.Moov.Traks[0].Tkhd.ModificationTime = macTime
		init.Moov.Traks[0].Mdia.Hdlr.Name = "agent " + utils.VERSION
		init.Moov.Traks[0].Mdia.Mdhd.Duration = mp4.VideoTotalDuration
		init.Moov.Traks[0].Mdia.Mdhd.CreationTime = macTime
		init.Moov.Traks[0].Mdia.Mdhd.ModificationTime = macTime
	case "H265", "HVC1":
		init.AddEmptyTrack(videoTimescale, "video", "und")
		includePS := true
		err := init.Moov.Traks[0].SetHEVCDescriptor("hvc1", mp4.VPSNALUs, mp4.SPSNALUs, mp4.PPSNALUs, [][]byte{}, includePS)
		if err != nil {
		}
		init.Moov.Traks[0].Tkhd.Duration = mp4.VideoTotalDuration
		init.Moov.Traks[0].Tkhd.Width = mp4ff.Fixed32(uint32(mp4.width) << 16)
		init.Moov.Traks[0].Tkhd.Height = mp4ff.Fixed32(uint32(mp4.height) << 16)
		init.Moov.Traks[0].Tkhd.CreationTime = macTime
		init.Moov.Traks[0].Tkhd.ModificationTime = macTime
		init.Moov.Traks[0].Mdia.Hdlr.Name = "agent " + utils.VERSION
		init.Moov.Traks[0].Mdia.Mdhd.Duration = mp4.VideoTotalDuration
		init.Moov.Traks[0].Mdia.Mdhd.CreationTime = macTime
		init.Moov.Traks[0].Mdia.Mdhd.ModificationTime = macTime
	}

	// Try adding audio track if available
	if mp4.AudioTrackName == "AAC" || mp4.AudioTrackName == "MP4A" {
		// Add an audio track to the moov box
		init.AddEmptyTrack(audioTimescale, "audio", "und")

		// Check if the same sample rate is set, otherwise we default to 48000
		audioSampleRate := 48000
		if config.Capture.IPCamera.SampleRate > 0 {
			audioSampleRate = config.Capture.IPCamera.SampleRate
		}
		// Set the audio descriptor
		err := init.Moov.Traks[1].SetAACDescriptor(29, audioSampleRate)
		if err != nil {
		}
		init.Moov.Traks[1].Tkhd.Duration = mp4.AudioTotalDuration
		init.Moov.Traks[1].Tkhd.CreationTime = macTime
		init.Moov.Traks[1].Tkhd.ModificationTime = macTime
		init.Moov.Traks[1].Mdia.Hdlr.Name = "agent " + utils.VERSION
		init.Moov.Traks[1].Mdia.Mdhd.Duration = mp4.AudioTotalDuration
		init.Moov.Traks[1].Mdia.Mdhd.CreationTime = macTime
		init.Moov.Traks[1].Mdia.Mdhd.ModificationTime = macTime
	}

	// Try adding subtitle track if available
	if mp4.VideoTrackName == "VTT" || mp4.VideoTrackName == "WebVTT" {
		// Add a subtitle track to the moov box
		init.AddEmptyTrack(videoTimescale, "subtitle", "und")
		// Set the subtitle descriptor
		err := init.Moov.Traks[2].SetWvttDescriptor("")
		if err != nil {
			//log.Log.Error("mp4.Close(): error setting VTT descriptor: " + err.Error())
			//return
		}
		init.Moov.Traks[2].Mdia.Hdlr.Name = "agent " + utils.VERSION
	}

	// We will create a fingerprint that's be encrypted with the public key, so we can verify the integrity of the file later.
	// The fingerprint will be a UUID box, which is a custom box that we can use to store the fingerprint.
	// Following fields are included in the fingerprint (UUID):
	// - Moov.Mvhd.CreationTime (the time the file was created)
	// - Moov.Mvhd.Duration (the total duration of the video)
	// - Moov.Trak.Hdlr.Name // (the name of the handler, which is the agent and version)
	// - len(Moof) // (the number of moof boxes in the file)
	// - size(Moof1) // (the size of the first moof box)
	// - size(Moof2) // (the size of the second moof box)
	// ..
	//
	// All attributes of the fingerprint are concatenated into a single string, which is then hashed using SHA-256
	// and encrypted with the public key.

	fingerprint := fmt.Sprintf("%d", init.Moov.Mvhd.CreationTime) + "_" +
		fmt.Sprintf("%d", init.Moov.Mvhd.Duration) + "_" +
		init.Moov.Trak.Mdia.Hdlr.Name + "_" +
		fmt.Sprintf("%d", mp4.MoofBoxes) + "_" // Number of moof boxes

	for i, size := range mp4.MoofBoxSizes {
		fingerprint += fmt.Sprintf("%d", size)
		if i < len(mp4.MoofBoxSizes)-1 {
			fingerprint += "_"
		}
	}
	// Remove trailing underscore if present
	if len(fingerprint) > 0 && fingerprint[len(fingerprint)-1] == '_' {
		fingerprint = fingerprint[:len(fingerprint)-1]
	}

	// Load the private key from the configuration
	privateKey := config.Signing.PrivateKey
	r := strings.NewReader(privateKey)
	pemBytes, _ := ioutil.ReadAll(r)
	block, _ := pem.Decode(pemBytes)

	if block == nil {
		//log.Log.Error("mp4.Close(): error decoding PEM block containing private key")
		//return
	} else {
		// Parse private key
		b := block.Bytes
		key, err := x509.ParsePKCS8PrivateKey(b)
		if err != nil {
			//log.Log.Error("mp4.Close(): error parsing private key: " + err.Error())
			//return
		} else {
			// Conver key to *rsa.PrivateKey
			rsaKey, _ := key.(*rsa.PrivateKey)
			fingerprintBytes := []byte(fingerprint)
			signature, err := encryption.SignWithPrivateKey(fingerprintBytes, rsaKey)
			if err == nil && len(signature) > 0 {
				uuid := &mp4ff.UUIDBox{}
				uuid.SetUUID("6b0c1f8e-3d2a-4f5b-9c7d-8f1e2b3c4d5e")
				uuid.UnknownPayload = signature
				init.Moov.AddChild(uuid)
			} else {
				//log.Log.Error("mp4.Close(): error signing fingerprint: " + err.Error())
			}
		}
	}

	// Build a Segment Index (sidx) box so players can seek directly to any
	// fragment without scanning the entire file.
	if len(mp4.SegmentDurations) > 0 {
		sidx := &mp4ff.SidxBox{
			Version:                  1,
			Flags:                    0,
			ReferenceID:              uint32(mp4.VideoTrack),
			Timescale:                videoTimescale,
			EarliestPresentationTime: 0,
			FirstOffset:              0,
			SidxRefs:                 make([]mp4ff.SidxRef, 0, len(mp4.SegmentDurations)),
		}
		for i, dur := range mp4.SegmentDurations {
			sidx.SidxRefs = append(sidx.SidxRefs, mp4ff.SidxRef{
				ReferenceType:      0, // media reference
				ReferencedSize:     uint32(mp4.MoofBoxSizes[i]),
				SubSegmentDuration: uint32(dur),
				StartsWithSAP:      1,
				SAPType:            1,
			})
		}
		init.AddChild(sidx)
	}

	// Encode the ftyp + moov + sidx into a buffer to measure the total size.
	// Then compute the correct sidx.FirstOffset (the gap between the end of
	// the sidx box and the first moof, occupied by the trailing free box)
	// and re-encode with the corrected value.
	var initBuf bytes.Buffer
	if err := init.Encode(&initBuf); err != nil {
		log.Log.Error("mp4.Close(): error encoding init segment: " + err.Error())
	}

	initSize := int64(initBuf.Len())

	// The sidx.FirstOffset is defined as the distance (in bytes) from the
	// anchor point (first byte after the sidx box) to the first byte of
	// the first referenced moof/mdat. Since sidx is the last box in init,
	// the anchor point is at initSize, and the first moof is at FreeBoxSize.
	if len(mp4.SegmentDurations) > 0 {
		if mp4.FreeBoxSize < initSize {
			// Avoid computing a negative offset and wrapping it to uint64.
			log.Log.Error("mp4.Close(): FreeBoxSize is smaller than initSize; skipping sidx FirstOffset adjustment")
		} else {
			firstOffset := uint64(mp4.FreeBoxSize - initSize)
			// Find the sidx we added and update its FirstOffset
			for _, child := range init.Children {
				if sidxBox, ok := child.(*mp4ff.SidxBox); ok {
					sidxBox.FirstOffset = firstOffset
					break
				}
			}
			// Re-encode with the corrected FirstOffset (same size, no layout change)
			initBuf.Reset()
			if err := init.Encode(&initBuf); err != nil {
				log.Log.Error("mp4.Close(): error re-encoding init segment: " + err.Error())
			}
			initSize = int64(initBuf.Len())
		}
	}

	if initSize > mp4.FreeBoxSize {
		log.Log.Error(fmt.Sprintf("mp4.Close(): init segment (%d bytes) exceeds reserved space (%d bytes), file may be corrupt", initSize, mp4.FreeBoxSize))
	}

	// Write the init segment at the beginning of the file, overwriting the free box placeholder.
	if _, err := mp4.FileWriter.WriteAt(initBuf.Bytes(), 0); err != nil {
		log.Log.Error("mp4.Close(): error writing init segment: " + err.Error())
	}

	// Fill any remaining reserved space with a new (smaller) free box so
	// the byte offsets of the moof/mdat boxes that follow are preserved.
	remainingSize := mp4.FreeBoxSize - initSize
	if remainingSize >= 8 { // minimum box size is 8 bytes (header only)
		newFree := mp4ff.NewFreeBox(make([]byte, remainingSize-8))
		var freeBuf bytes.Buffer
		if err := newFree.Encode(&freeBuf); err != nil {
			log.Log.Error("mp4.Close(): error encoding free box: " + err.Error())
		}
		if _, err := mp4.FileWriter.WriteAt(freeBuf.Bytes(), initSize); err != nil {
			log.Log.Error("mp4.Close(): error writing free box: " + err.Error())
		}
	}

	if err := mp4.FileWriter.Sync(); err != nil {
		log.Log.Error("mp4.Close(): error syncing file: " + err.Error())
	}
	mp4.FileWriter.Close()
}

// annexBToLengthPrefixed converts Annex B formatted H264 data (with start codes)
// into length-prefixed NAL units (4-byte length before each NAL unit).
func annexBToLengthPrefixed(data []byte) ([]byte, error) {
	var out bytes.Buffer

	// Find start codes and split NAL units
	nalus := splitNALUs(data)
	if len(nalus) == 0 {
		return nil, fmt.Errorf("no NAL units found")
	}

	for _, nalu := range nalus {
		// Remove Annex B start codes (0x000001 or 0x00000001) from the beginning of each NALU
		nalu = removeAnnexBStartCode(nalu)
		if len(nalu) == 0 {
			continue
		}
		// Write 4-byte big-endian length
		length := uint32(len(nalu))
		lenBytes := []byte{
			byte(length >> 24),
			byte(length >> 16),
			byte(length >> 8),
			byte(length),
		}
		out.Write(lenBytes)
		out.Write(nalu)
	}

	return out.Bytes(), nil
}

// removeAnnexBStartCode removes a leading Annex B start code from a NALU if present.
func removeAnnexBStartCode(nalu []byte) []byte {
	if len(nalu) >= 4 && nalu[0] == 0x00 && nalu[1] == 0x00 {
		if nalu[2] == 0x01 {
			return nalu[3:]
		}
		if nalu[2] == 0x00 && nalu[3] == 0x01 {
			return nalu[4:]
		}
	}
	return nalu
}

// splitNALUs splits Annex B data into raw NAL units without start codes.
func splitNALUs(data []byte) [][]byte {
	var nalus [][]byte
	start := 0

	for start < len(data) {
		// Find next start code (0x000001 or 0x00000001)
		i := findStartCode(data, start+3)
		if i < 0 {
			// Last NALU till end of data
			nalus = append(nalus, data[start:])
			break
		}
		// NAL unit is between start and i
		nalus = append(nalus, data[start:i])
		start = i
	}

	return nalus
}

// findStartCode returns the index of the next Annex B start code (0x000001 or 0x00000001) after pos, or -1 if none.
func findStartCode(data []byte, pos int) int {
	for i := pos; i+3 < len(data); i++ {
		if data[i] == 0x00 && data[i+1] == 0x00 {
			if data[i+2] == 0x01 {
				return i
			}
			if i+3 < len(data) && data[i+2] == 0x00 && data[i+3] == 0x01 {
				return i
			}
		}
	}
	return -1
}

// FindSyncword searches for the AAC syncword (0xFFF0) in the given byte slice starting from the specified offset.
func FindSyncword(aac []byte, offset int) int {
	for i := offset; i < len(aac)-1; i++ {
		if aac[i] == 0xFF && aac[i+1]&0xF0 == 0xF0 {
			return i
		}
	}
	return -1
}

// Table 31 – Profiles
// index      profile
//   0        Main profile
//   1        Low Complexity profile (LC)
//   2        Scalable Sampling Rate profile (SSR)
//   3        (reserved)

type START_CODE_TYPE int

const (
	START_CODE_3 START_CODE_TYPE = 3
	START_CODE_4 START_CODE_TYPE = 4
)

func FindStartCode(nalu []byte, offset int) (int, START_CODE_TYPE) {
	idx := bytes.Index(nalu[offset:], []byte{0x00, 0x00, 0x01})
	switch {
	case idx > 0:
		if nalu[offset+idx-1] == 0x00 {
			return offset + idx - 1, START_CODE_4
		}
		fallthrough
	case idx == 0:
		return offset + idx, START_CODE_3
	}
	return -1, START_CODE_3
}

func SplitFrame(frames []byte, onFrame func(nalu []byte) bool) {
	beg, sc := FindStartCode(frames, 0)
	for beg >= 0 {
		end, sc2 := FindStartCode(frames, beg+int(sc))
		if end == -1 {
			if onFrame != nil {
				onFrame(frames[beg+int(sc):])
			}
			break
		}
		if onFrame != nil && onFrame(frames[beg+int(sc):end]) == false {
			break
		}
		beg = end
		sc = sc2
	}
}

func SplitFrameWithStartCode(frames []byte, onFrame func(nalu []byte) bool) {
	beg, sc := FindStartCode(frames, 0)
	for beg >= 0 {
		end, sc2 := FindStartCode(frames, beg+int(sc))
		if end == -1 {
			if onFrame != nil && (beg+int(sc)) < len(frames) {
				onFrame(frames[beg:])
			}
			break
		}
		if onFrame != nil && (beg+int(sc)) < end && onFrame(frames[beg:end]) == false {
			break
		}
		beg = end
		sc = sc2
	}
}

func SplitAACFrame(frames []byte, onFrame func(started bool, aac []byte)) {
	var adts ADTS_Frame_Header
	start := FindSyncword(frames, 0)
	started := false
	for start >= 0 {
		adts.Decode(frames[start:])
		onFrame(started, frames[start:start+int(adts.Variable_Header.Frame_length)])
		start = FindSyncword(frames, start+int(adts.Variable_Header.Frame_length))
		started = true
	}
}

type AAC_PROFILE int

const (
	MAIN AAC_PROFILE = iota
	LC
	SSR
)

type AAC_SAMPLING_FREQUENCY int

const (
	AAC_SAMPLE_96000 AAC_SAMPLING_FREQUENCY = iota
	AAC_SAMPLE_88200
	AAC_SAMPLE_64000
	AAC_SAMPLE_48000
	AAC_SAMPLE_44100
	AAC_SAMPLE_32000
	AAC_SAMPLE_24000
	AAC_SAMPLE_22050
	AAC_SAMPLE_16000
	AAC_SAMPLE_12000
	AAC_SAMPLE_11025
	AAC_SAMPLE_8000
	AAC_SAMPLE_7350
)

var AAC_Sampling_Idx [13]int = [13]int{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350}

// Table 4 – Syntax of adts_sequence()
// adts_sequence() {
//         while (nextbits() == syncword) {
//             adts_frame();
//         }
// }
// Table 5 – Syntax of adts_frame()
// adts_frame() {
//     adts_fixed_header();
//     adts_variable_header();
//     if (number_of_raw_data_blocks_in_frame == 0) {
//         adts_error_check();
//         raw_data_block();
//     }
//     else {
//         adts_header_error_check();
//         for (i = 0; i <= number_of_raw_data_blocks_in_frame;i++ {
//             raw_data_block();
//             adts_raw_data_block_error_check();
//         }
//     }
// }

// adts_fixed_header()
// {
//         syncword;                         12           bslbf
//         ID;                                1            bslbf
//         layer;                          2            uimsbf
//         protection_absent;              1            bslbf
//         profile;                        2            uimsbf
//         sampling_frequency_index;       4            uimsbf
//         private_bit;                    1            bslbf
//         channel_configuration;          3            uimsbf
//         original/copy;                  1            bslbf
//         home;                           1            bslbf
// }

type ADTS_Fix_Header struct {
	ID                       uint8
	Layer                    uint8
	Protection_absent        uint8
	Profile                  uint8
	Sampling_frequency_index uint8
	Private_bit              uint8
	Channel_configuration    uint8
	Originalorcopy           uint8
	Home                     uint8
}

// adts_variable_header() {
//      copyright_identification_bit;               1      bslbf
//      copyright_identification_start;             1      bslbf
//      frame_length;                               13     bslbf
//      adts_buffer_fullness;                       11     bslbf
//      number_of_raw_data_blocks_in_frame;         2      uimsfb
// }

type ADTS_Variable_Header struct {
	Copyright_identification_bit       uint8
	copyright_identification_start     uint8
	Frame_length                       uint16
	Adts_buffer_fullness               uint16
	Number_of_raw_data_blocks_in_frame uint8
}

type ADTS_Frame_Header struct {
	Fix_Header      ADTS_Fix_Header
	Variable_Header ADTS_Variable_Header
}

func NewAdtsFrameHeader() *ADTS_Frame_Header {
	return &ADTS_Frame_Header{
		Fix_Header: ADTS_Fix_Header{
			ID:                       0,
			Layer:                    0,
			Protection_absent:        1,
			Profile:                  uint8(MAIN),
			Sampling_frequency_index: uint8(AAC_SAMPLE_44100),
			Private_bit:              0,
			Channel_configuration:    0,
			Originalorcopy:           0,
			Home:                     0,
		},

		Variable_Header: ADTS_Variable_Header{
			copyright_identification_start:     0,
			Copyright_identification_bit:       0,
			Frame_length:                       0,
			Adts_buffer_fullness:               0,
			Number_of_raw_data_blocks_in_frame: 0,
		},
	}
}

func (frame *ADTS_Frame_Header) Decode(aac []byte) {
	_ = aac[6]
	frame.Fix_Header.ID = aac[1] >> 3
	frame.Fix_Header.Layer = aac[1] >> 1 & 0x03
	frame.Fix_Header.Protection_absent = aac[1] & 0x01
	frame.Fix_Header.Profile = aac[2] >> 6 & 0x03
	frame.Fix_Header.Sampling_frequency_index = aac[2] >> 2 & 0x0F
	frame.Fix_Header.Private_bit = aac[2] >> 1 & 0x01
	frame.Fix_Header.Channel_configuration = (aac[2] & 0x01 << 2) | (aac[3] >> 6)
	frame.Fix_Header.Originalorcopy = aac[3] >> 5 & 0x01
	frame.Fix_Header.Home = aac[3] >> 4 & 0x01
	frame.Variable_Header.Copyright_identification_bit = aac[3] >> 3 & 0x01
	frame.Variable_Header.copyright_identification_start = aac[3] >> 2 & 0x01
	frame.Variable_Header.Frame_length = (uint16(aac[3]&0x03) << 11) | (uint16(aac[4]) << 3) | (uint16(aac[5]>>5) & 0x07)
	frame.Variable_Header.Adts_buffer_fullness = (uint16(aac[5]&0x1F) << 6) | uint16(aac[6]>>2)
	frame.Variable_Header.Number_of_raw_data_blocks_in_frame = aac[6] & 0x03
}

func (frame *ADTS_Frame_Header) Encode() []byte {
	var hdr []byte
	if frame.Fix_Header.Protection_absent == 1 {
		hdr = make([]byte, 7)
	} else {
		hdr = make([]byte, 9)
	}
	hdr[0] = 0xFF
	hdr[1] = 0xF0
	hdr[1] = hdr[1] | (frame.Fix_Header.ID << 3) | (frame.Fix_Header.Layer << 1) | frame.Fix_Header.Protection_absent
	hdr[2] = frame.Fix_Header.Profile<<6 | frame.Fix_Header.Sampling_frequency_index<<2 | frame.Fix_Header.Private_bit<<1 | frame.Fix_Header.Channel_configuration>>2
	hdr[3] = frame.Fix_Header.Channel_configuration<<6 | frame.Fix_Header.Originalorcopy<<5 | frame.Fix_Header.Home<<4
	hdr[3] = hdr[3] | frame.Variable_Header.copyright_identification_start<<3 | frame.Variable_Header.Copyright_identification_bit<<2 | byte(frame.Variable_Header.Frame_length<<11)
	hdr[4] = byte(frame.Variable_Header.Frame_length >> 3)
	hdr[5] = byte((frame.Variable_Header.Frame_length&0x07)<<5) | byte(frame.Variable_Header.Adts_buffer_fullness>>3)
	hdr[6] = byte(frame.Variable_Header.Adts_buffer_fullness&0x3F<<2) | frame.Variable_Header.Number_of_raw_data_blocks_in_frame
	return hdr
}

func SampleToAACSampleIndex(sampling int) int {
	for i, v := range AAC_Sampling_Idx {
		if v == sampling {
			return i
		}
	}
	return -1
}

func AACSampleIdxToSample(idx int) int {
	return AAC_Sampling_Idx[idx]
}

// +--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+
// |  audio object type(5 bits)  |  sampling frequency index(4 bits) |   channel configuration(4 bits)  | GA framelength flag(1 bits) |  GA Depends on core coder(1 bits) | GA Extension Flag(1 bits) |
// +--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------+

type AudioSpecificConfiguration struct {
	Audio_object_type        uint8
	Sample_freq_index        uint8
	Channel_configuration    uint8
	GA_framelength_flag      uint8
	GA_depends_on_core_coder uint8
	GA_extension_flag        uint8
}

func NewAudioSpecificConfiguration() *AudioSpecificConfiguration {
	return &AudioSpecificConfiguration{
		Audio_object_type:        0,
		Sample_freq_index:        0,
		Channel_configuration:    0,
		GA_framelength_flag:      0,
		GA_depends_on_core_coder: 0,
		GA_extension_flag:        0,
	}
}

func (asc *AudioSpecificConfiguration) Encode() []byte {
	buf := make([]byte, 2)
	buf[0] = (asc.Audio_object_type & 0x1f << 3) | (asc.Sample_freq_index & 0x0F >> 1)
	buf[1] = (asc.Sample_freq_index & 0x0F << 7) | (asc.Channel_configuration & 0x0F << 3) | (asc.GA_framelength_flag & 0x01 << 2) | (asc.GA_depends_on_core_coder & 0x01 << 1) | (asc.GA_extension_flag & 0x01)
	return buf
}

func (asc *AudioSpecificConfiguration) Decode(buf []byte) error {

	if len(buf) < 2 {
		return errors.New("len of buf < 2 ")
	}

	asc.Audio_object_type = buf[0] >> 3
	asc.Sample_freq_index = (buf[0] & 0x07 << 1) | (buf[1] >> 7)
	asc.Channel_configuration = buf[1] >> 3 & 0x0F
	asc.GA_framelength_flag = buf[1] >> 2 & 0x01
	asc.GA_depends_on_core_coder = buf[1] >> 1 & 0x01
	asc.GA_extension_flag = buf[1] & 0x01
	return nil
}

func ConvertADTSToASC(frame []byte) (*AudioSpecificConfiguration, error) {
	if len(frame) < 7 {
		return nil, errors.New("len of frame < 7")
	}
	adts := NewAdtsFrameHeader()
	adts.Decode(frame)
	asc := NewAudioSpecificConfiguration()
	asc.Audio_object_type = adts.Fix_Header.Profile + 1
	asc.Channel_configuration = adts.Fix_Header.Channel_configuration
	asc.Sample_freq_index = adts.Fix_Header.Sampling_frequency_index
	return asc, nil
}

func ConvertASCToADTS(asc []byte, aacbytes int) (*ADTS_Frame_Header, error) {
	aac_asc := NewAudioSpecificConfiguration()
	err := aac_asc.Decode(asc)
	if err != nil {
		return nil, err
	}
	aac_adts := NewAdtsFrameHeader()
	aac_adts.Fix_Header.Profile = aac_asc.Audio_object_type - 1
	aac_adts.Fix_Header.Channel_configuration = aac_asc.Channel_configuration
	aac_adts.Fix_Header.Sampling_frequency_index = aac_asc.Sample_freq_index
	aac_adts.Fix_Header.Protection_absent = 1
	aac_adts.Variable_Header.Adts_buffer_fullness = 0x3F
	aac_adts.Variable_Header.Frame_length = uint16(aacbytes)
	return aac_adts, nil
}
