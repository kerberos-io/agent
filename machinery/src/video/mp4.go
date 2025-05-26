package video

import (
	"os"
	"time"

	"github.com/Eyevinn/mp4ff/mp4"
	mp4ff "github.com/Eyevinn/mp4ff/mp4"
)

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
	init := mp4.CreateEmptyInit()
	init.AddEmptyTrack(videoTimescale, "video", "und")

	init.Moov.Mvex.AddChild(&mp4.MehdBox{FragmentDuration: int64(450000)})

	trak := init.Moov.Trak
	includePS := true
	err := trak.SetAVCDescriptor("avc1", spsNALUs, ppsNALUs, includePS)
	if err != nil {
		panic(err)
	}
	trak.Tkhd.Duration = 450000 // 5 seconds

	// Add an ELST box to the track
	elst := &mp4ff.ElstBox{
		Version: 0,
		Flags:   0,
		Entries: []mp4ff.ElstEntry{
			{
				SegmentDuration:   450000, // 5 seconds
				MediaTime:         0,
				MediaRateInteger:  1,
				MediaRateFraction: 0,
			},
		},
	}
	init.Moov.Trak.AddChild(&mp4ff.EdtsBox{
		Elst: []*mp4ff.ElstBox{elst},
	})

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

	// Set Stts
	init.Moov.Trak.Mdia.Minf.Stbl.Stts.SampleCount = []uint32{124}
	init.Moov.Trak.Mdia.Minf.Stbl.Stts.SampleTimeDelta = []uint32{90000} // 1 second in 90kHz timescale

	// Set Stsc
	init.Moov.Trak.Mdia.Minf.Stbl.Stsc.Version = 0
	init.Moov.Trak.Mdia.Minf.Stbl.Stsc.Flags = 0
	init.Moov.Trak.Mdia.Minf.Stbl.Stsc.SampleDescriptionID = []uint32{1}
	init.Moov.Trak.Mdia.Minf.Stbl.Stsc.Entries = []mp4ff.StscEntry{
		{
			FirstChunk:      1,
			SamplesPerChunk: 124,
		},
	}

	// Set the compressorName
	//trak.Mdia.

	err = init.Encode(ofd)

	sidxBox := mp4ff.CreateSidx(0)
	sidxBox.Timescale = videoTimescale
	err = sidxBox.Encode(ofd)

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

	if isKeyframe {

		// Write the segment to the file
		if mp4.Start {
			err := mp4.Segment.Encode(mp4.Writer)
			if err != nil {
				panic(err)
			}
		}

		mp4.Start = true

		// Create a new media segment
		seg := mp4ff.NewMediaSegment()
		frag, err := mp4ff.CreateFragment(uint32(mp4.SegmentCount), trackID)
		if err != nil {
			panic(err)
		}
		seg.AddFragment(frag)

		// Increment the segment count
		mp4.SegmentCount = mp4.SegmentCount + 1

		// Set to MP4 struct
		mp4.Segment = seg
		mp4.Fragment = frag

		// Set the start PTS for the next segment
		mp4.StartPTS = pts
	}

	var fullSample mp4ff.FullSample
	// Set the sample data
	duration = duration * 90 // Convert duration to 90kHz timescale
	mp4.TotalDuration += duration
	fullSample.Data = data
	fullSample.DecodeTime = mp4.TotalDuration
	fullSample.Sample = mp4ff.Sample{
		Dur:                   uint32(duration),
		Size:                  uint32(len(data)),
		Flags:                 0,
		CompositionTimeOffset: 0,
	}

	// Add a sample to the track
	// This is a placeholder function
	// In a real implementation, this would add a sample to the track
	err := mp4.Fragment.AddFullSampleToTrack(fullSample, trackID)
	if err != nil {
		panic(err)
	}
}

func (mp4 *MP4) Close() {
	err := mp4.Segment.Encode(mp4.Writer)
	if err != nil {
		panic(err)
	}
	defer mp4.Writer.Close()
}
