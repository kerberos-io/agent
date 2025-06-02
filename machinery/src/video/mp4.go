package video

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Eyevinn/mp4ff/mp4"
	mp4ff "github.com/Eyevinn/mp4ff/mp4"
	"github.com/kerberos-io/agent/machinery/src/utils"
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
	FileWriter    *os.File
	Writer        *bufio.Writer
	SegmentCount  int
	SampleCount   int
	StartPTS      uint64
	TotalDuration uint64
	Start         bool
	SPSNALUs      [][]byte // SPS NALUs for H264
	PPSNALUs      [][]byte // PPS NALUs for H264
	FreeBoxSize   int64
	MoofBoxes     int64   // Number of moof boxes in the file
	MoofBoxSizes  []int64 // Sizes of each moof box
}

// NewMP4 creates a new MP4 object
func NewMP4(fileName string, spsNALUs [][]byte, ppsNALUs [][]byte) *MP4 {

	init := mp4ff.NewMP4Init()

	// Add a free box to the init segment
	// Prepend a free box to the init segment with a size of 1000
	freeBoxSize := 2048
	free := mp4ff.NewFreeBox(make([]byte, freeBoxSize))
	init.AddChild(free)

	// Create a writer
	ofd, err := os.Create(fileName)
	if err != nil {
		panic(err)
	}

	// Create a buffered writer
	bufferedWriter := bufio.NewWriterSize(ofd, 64*1024) // 64KB buffer

	// We will write the empty init segment to the file
	// so we can overwrite it later with the actual init segment.
	err = init.Encode(bufferedWriter)
	if err != nil {
		panic(err)
	}

	return &MP4{
		FileName:    fileName,
		FreeBoxSize: int64(freeBoxSize),
		Init:        init,
		FileWriter:  ofd,
		Writer:      bufferedWriter,
		SPSNALUs:    spsNALUs,
		PPSNALUs:    ppsNALUs,
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
	mp4.TrackIDs = append(mp4.TrackIDs, 1) // Example track ID
}

// AddAudioTrack
// Add an audio track to the MP4 file
func (mp4 *MP4) AddAudioTrack(codec string) {
	// Add an audio track to the MP4 file
	// This is a placeholder function
	// In a real implementation, this would add an audio track to the MP4 file
	mp4.TrackIDs = append(mp4.TrackIDs, 2) // Example track ID
}

func (mp4 *MP4) AddMediaSegment(segNr int) {
}

func (mp4 *MP4) AddSampleToTrack(trackID uint32, isKeyframe bool, data []byte, pts uint64, duration uint64) {

	lengthPrefixed, err := annexBToLengthPrefixed(data)
	var fullSample mp4ff.FullSample
	if err == nil {
		// Set the sample dat
		// a
		flags := uint32(33554432)
		if !isKeyframe {
			flags = uint32(16842752)
		}

		duration = duration * 90 // Convert duration to 90kHz timescale
		fmt.Printf("Adding sample to track %d, PTS: %d, Duration: %d, size: %d, Keyframe: %t\n", trackID, pts, duration, len(lengthPrefixed), isKeyframe)
		mp4.TotalDuration += duration
		fullSample.Data = lengthPrefixed
		fullSample.DecodeTime = mp4.TotalDuration - duration
		fullSample.Sample = mp4ff.Sample{
			Dur:   uint32(duration),
			Size:  uint32(len(fullSample.Data)),
			Flags: flags,
		}

		if isKeyframe {

			// Write the segment to the file
			if mp4.Start {
				mp4.MoofBoxes = mp4.MoofBoxes + 1
				mp4.MoofBoxSizes = append(mp4.MoofBoxSizes, int64(mp4.Segment.Size()))
				err := mp4.Segment.Encode(mp4.Writer)
				if err != nil {
					panic(err)
				}
			}

			mp4.Start = true

			// Increment the segment count
			mp4.SegmentCount = mp4.SegmentCount + 1

			// Create a new media segment
			seg := mp4ff.NewMediaSegment()
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
	mp4.Writer.Flush()
	defer mp4.FileWriter.Close()

	// Now we have all the moof and mdat boxes written to the file.
	// We can now generate the ftyp and moov boxes, and replace it with the free box we added earlier (size of 10008 bytes).
	mp4.Init = mp4ff.NewMP4Init()
	mp4.Init.Moov = mp4ff.NewMoovBox()
	majorBrand := "isom"
	minorVersion := uint32(512)
	compatibleBrands := []string{"iso2", "avc1", "mp41"}
	ftyp := mp4ff.NewFtyp(majorBrand, minorVersion, compatibleBrands)
	mp4.Init.AddChild(ftyp)
	moov := mp4ff.NewMoovBox()

	// Set the creation time and modification time for the moov box
	mdhd := mp4ff.MdhdBox{
		Version:          0,
		Flags:            0,
		CreationTime:     uint64(time.Now().Unix()),
		ModificationTime: uint64(time.Now().Unix()),
		Timescale:        90000,        // 90kHz timescale
		Language:         uint16(0x55), // Undetermined language (und)
	}
	moov.AddChild(&mdhd)
	mp4.Init.AddChild(moov)

	// Add the video track to the moov box
	videoTimescale := uint32(90000) // 90kHz timescale for video
	mp4.Init.AddEmptyTrack(videoTimescale, "video", "und")
	mp4.Init.Ftyp.AddCompatibleBrands([]string{"isom", "iso2", "avc1", "mp41"})

	trak := mp4.Init.Moov.Trak
	includePS := true
	err = trak.SetAVCDescriptor("avc1", mp4.SPSNALUs, mp4.PPSNALUs, includePS)
	if err != nil {
		panic(err)
	}

	// Override the HandlerBox, and more specifically the name field with "agent and version"
	mp4.Init.Moov.Trak.Mdia.Hdlr.Name = "agent " + utils.VERSION

	// Set the total duration in the moov box
	mp4.Init.Moov.Mvhd.Duration = mp4.TotalDuration
	mp4.Init.Moov.Mvex.AddChild(&mp4ff.MehdBox{FragmentDuration: int64(mp4.TotalDuration)})
	mp4.Init.Moov.Trak.Tkhd.Duration = mp4.TotalDuration

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

	fingerprint := fmt.Sprintf("%d", mp4.Init.Moov.Mvhd.CreationTime) +
		fmt.Sprintf("%d", mp4.Init.Moov.Mvhd.Duration) +
		mp4.Init.Moov.Trak.Mdia.Hdlr.Name +
		fmt.Sprintf("%d", mp4.MoofBoxes) // Number of moof boxes

	uuid := &mp4ff.UUIDBox{}
	uuid.SetUUID("6b0c1f8e-3d2a-4f5b-9c7d-8f1e2b3c4d5e")
	uuid.UnknownPayload = []byte(fingerprint)
	moov.AddChild(uuid)

	mvhd := mp4ff.CreateMvhd()
	moov.AddChild(mvhd)
	mvex := mp4ff.NewMvexBox()
	moov.AddChild(mvex)

	// Get a bit slice writer for the init segment
	// Get a byte buffer of 10008 bytes to write the init segment
	buffer := bytes.NewBuffer(make([]byte, 0))
	mp4.Init.Encode(buffer)

	// The first 10008 bytes of the file is a free box, so we can read it and replace it with the moov box.
	// The init box might not be 10008 bytes, so we need to read the first 10008 bytes and then replace it with the moov box.
	// while the remaining bytes are for a new free box.
	// Write the init segment at the beginning of the file, replacing the free box
	if _, err := mp4.FileWriter.WriteAt(buffer.Bytes(), 0); err != nil {
		panic(err)
	}

	remainingSize := mp4.FreeBoxSize - int64(buffer.Len())
	if remainingSize > 0 {
		newFreeBox := mp4ff.NewFreeBox(make([]byte, remainingSize))
		var freeBuf bytes.Buffer
		if err := newFreeBox.Encode(&freeBuf); err != nil {
			panic(err)
		}
		if _, err := mp4.FileWriter.WriteAt(freeBuf.Bytes(), int64(buffer.Len())); err != nil {
			panic(err)
		}
	}

	/*
		err = mp4Root.UpdateSidx(addIfNotExists, false)
	*/
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
