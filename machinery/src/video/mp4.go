package video

import (
	"bytes"
	"os"

	"github.com/Eyevinn/mp4ff/mp4"
	mp4ff "github.com/Eyevinn/mp4ff/mp4"
)

type MP4 struct {
	// FileName is the name of the file
	FileName string
	width    int
	height   int
	Init     *mp4.InitSegment
	Segment  *mp4ff.MediaSegment
	Fragment *mp4ff.Fragment
	TrackIDs []uint32
}

// NewMP4 creates a new MP4 object
func NewMP4(fileName string, spsNALUs [][]byte, ppsNALUs [][]byte) *MP4 {

	videoTimescale := uint32(180000)
	init := mp4.CreateEmptyInit()
	init.AddEmptyTrack(videoTimescale, "video", "und")

	trak := init.Moov.Trak
	includePS := true
	err := trak.SetAVCDescriptor("avc1", spsNALUs, ppsNALUs, includePS)
	if err != nil {
		panic(err)
	}

	// We set the trackIDs (should be dynamic)
	trackIDs := []uint32{0}

	return &MP4{
		FileName: fileName,
		TrackIDs: trackIDs,
		Init:     init,
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
	seg := mp4ff.NewMediaSegment()
	frag, err := mp4ff.CreateMultiTrackFragment(uint32(segNr), mp4.TrackIDs)
	if err != nil {
		panic(err)
	}

	seg.AddFragment(frag)

	// Set to MP4 struct
	mp4.Segment = seg
	mp4.Fragment = frag
}

func (mp4 *MP4) AddSampleToTrack(trackID uint32, data []byte, pts uint64, duration uint64) {

	var fullSample mp4ff.FullSample
	// Set the sample data
	fullSample.Data = data
	fullSample.DecodeTime = pts
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
	// Open file for writing
	ofd, err := os.Create(mp4.FileName)
	if err != nil {
		panic(err)
	}
	defer ofd.Close()

	initBuf := &bytes.Buffer{}
	err = mp4.Init.Encode(initBuf)
	if err != nil {
		panic(err)
	}

	// Write Init segment to file
	_, err = ofd.Write(initBuf.Bytes())
	if err != nil {
		panic(err)
	}

	if mp4.Segment != nil {

		// Open file for writing
		ofd_m4s, err := os.Create(mp4.FileName + ".m4s")
		if err != nil {
			panic(err)
		}
		defer ofd_m4s.Close()

		segBuf := &bytes.Buffer{}
		err = mp4.Segment.Encode(segBuf)
		if err != nil {
			panic(err)
		}
		_, err = ofd_m4s.Write(segBuf.Bytes())
		if err != nil {
			panic(err)
		}
	}
}
