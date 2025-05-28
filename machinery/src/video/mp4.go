package video

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Eyevinn/mp4ff/mp4"
	mp4ff "github.com/Eyevinn/mp4ff/mp4"
)

var LastPTS uint64 = 0 // Last PTS for the current segment

type MP4 struct {
	// FileName is the name of the file
	FileName      string
	width         int
	height        int
	Init          *mp4.InitSegment
	Segment       *mp4ff.MediaSegment
	Fragment      *mp4ff.Fragment
	TrackIDs      []uint32
	Writer        *os.File
	SegmentCount  int
	SampleCount   int
	StartPTS      uint64
	TotalDuration uint64
	Start         bool
}

// NewMP4 creates a new MP4 object
func NewMP4(fileName string, spsNALUs [][]byte, ppsNALUs [][]byte) *MP4 {

	videoTimescale := uint32(90000)

	init := mp4ff.NewMP4Init()

	// Set the major brand, minor version, and compatible brands
	majorBrand := "isom"
	minorVersion := uint32(512)
	compatibleBrands := []string{"iso2", "avc1", "mp41"}
	ftyp := mp4ff.NewFtyp(majorBrand, minorVersion, compatibleBrands)
	init.AddChild(ftyp)
	moov := mp4ff.NewMoovBox()
	init.AddChild(moov)
	mvhd := mp4ff.CreateMvhd()
	moov.AddChild(mvhd)
	mvex := mp4ff.NewMvexBox()
	moov.AddChild(mvex)

	init.AddEmptyTrack(videoTimescale, "video", "und")

	init.Ftyp.AddCompatibleBrands([]string{"isom", "iso2", "avc1", "mp41"})
	init.Moov.Mvex.AddChild(&mp4.MehdBox{FragmentDuration: int64(900000)})

	trak := init.Moov.Trak
	includePS := true
	err := trak.SetAVCDescriptor("avc1", spsNALUs, ppsNALUs, includePS)
	if err != nil {
		panic(err)
	}

	// We set the trackIDs (should be dynamic)
	trackIDs := []uint32{1}

	// Create a writer
	ofd, err := os.Create(fileName)
	if err != nil {
		panic(err)
	}

	// Set estimated duration
	init.Moov.Mvhd.Duration = 450000 // 5 seconds

	// Set the creation time
	init.Moov.Mvhd.SetCreationTimeS(time.Now().Unix())
	// Set the modification time
	init.Moov.Mvhd.SetModificationTimeS(time.Now().Unix())
	err = init.Encode(ofd)
	if err != nil {
		panic(err)
	}

	sidxBox := mp4ff.CreateSidx(0)
	sidxBox.Timescale = videoTimescale
	//err = sidxBox.Encode(ofd)

	// Add sidx box

	if err != nil {
		panic(err)
	}

	return &MP4{
		FileName: fileName,
		TrackIDs: trackIDs,
		Init:     init,
		Writer:   ofd,
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
func (mp4 *MP4) AddVideoTrack(codec string) {
	// Add a video track to the MP4 file
	// This is a placeholder function
	// In a real implementation, this would add a video track to the MP4 file
}

// AddAudioTrack
// Add an audio track to the MP4 file
func (mp4 *MP4) AddAudioTrack(codec string) {
	// Add an audio track to the MP4 file
	// This is a placeholder function
	// In a real implementation, this would add an audio track to the MP4 file
}

func (mp4 *MP4) AddMediaSegment(segNr int) {
}

func (mp4 *MP4) AddSampleToTrack(trackID uint32, isKeyframe bool, data []byte, pts uint64, duration uint64) {

	//if pts == 0 {
	//	return
	//}

	lengthPrefixed, err := annexBToLengthPrefixed(data)
	var fullSample mp4ff.FullSample
	if err == nil {
		// Set the sample dat
		// a
		duration = duration * 90 // Convert duration to 90kHz timescale
		fmt.Printf("Adding sample to track %d, PTS: %d, Duration: %d, size: %d, Keyframe: %t\n", trackID, pts, duration, len(lengthPrefixed), isKeyframe)
		mp4.TotalDuration += duration
		fullSample.Data = lengthPrefixed
		fullSample.DecodeTime = mp4.TotalDuration - duration
		fullSample.Sample = mp4ff.Sample{
			Dur:  uint32(duration),
			Size: uint32(len(fullSample.Data)),
		}

		if isKeyframe {

			// Write the segment to the file
			if mp4.Start {
				err := mp4.Segment.Encode(mp4.Writer)
				if err != nil {
					panic(err)
				}
			}

			mp4.Start = true

			// Increment the segment count
			mp4.SegmentCount = mp4.SegmentCount + 1

			// Create a new media segment
			seg := mp4ff.NewMediaSegmentWithoutStyp()
			frag, err := mp4ff.CreateFragment(uint32(mp4.SegmentCount), trackID)
			if err != nil {
				panic(err)
			}
			seg.AddFragment(frag)

			// Set to MP4 struct
			mp4.Segment = seg
			mp4.Fragment = frag

			// Set the start PTS for the next segment
			mp4.StartPTS = pts
		}

		if mp4.Start {

			// Add a sample to the track
			// This is a placeholder function
			// In a real implementation, this would add a sample to the track
			err = mp4.Fragment.AddFullSampleToTrack(fullSample, trackID)
			if err != nil {
				log.Printf("Error adding sample to track %d: %v", trackID, err)
				return
			}
			LastPTS = pts
		}
	}
}

func (mp4 *MP4) Close() {
	err := mp4.Segment.Encode(mp4.Writer)
	if err != nil {
		panic(err)
	}
	defer mp4.Writer.Close()
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
