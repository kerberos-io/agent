package video

import (
	"bufio"
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	mp4ff "github.com/Eyevinn/mp4ff/mp4"
	"github.com/kerberos-io/agent/machinery/src/encryption"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/utils"
)

var LastPTS uint64 = 0 // Last PTS for the current segment

type MP4 struct {
	// FileName is the name of the file
	FileName      string
	width         int
	height        int
	Segments      []*mp4ff.MediaSegment // List of media segments
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
	StartTime     uint64  // Start time of the MP4 file
	VideoTrack    int     // Track ID for the video track
	AudioTrack    int     // Track ID for the audio track
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
		StartTime:   uint64(time.Now().Unix()),
		FreeBoxSize: int64(freeBoxSize),
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
	nextTrack := uint32(len(mp4.TrackIDs) + 1)
	mp4.VideoTrack = int(nextTrack)
	mp4.TrackIDs = append(mp4.TrackIDs, nextTrack)
}

// AddAudioTrack
// Add an audio track to the MP4 file
func (mp4 *MP4) AddAudioTrack(codec string) {
	nextTrack := uint32(len(mp4.TrackIDs) + 1)
	mp4.AudioTrack = int(nextTrack)
	mp4.TrackIDs = append(mp4.TrackIDs, nextTrack)
}

func (mp4 *MP4) AddMediaSegment(segNr int) {
}

func (mp4 *MP4) AddSampleToTrack(trackID uint32, isKeyframe bool, data []byte, pts uint64, duration uint64) error {

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
					return err
				}
				mp4.Segments = append(mp4.Segments, mp4.Segment)
			}

			mp4.Start = true

			// Increment the segment count
			mp4.SegmentCount = mp4.SegmentCount + 1

			// Create a new media segment
			seg := mp4ff.NewMediaSegment()
			frag, err := mp4ff.CreateFragment(uint32(mp4.SegmentCount), trackID)
			if err != nil {
				return err
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
				return err
			}
			LastPTS = pts
		}
	} else {
		log.Printf("Error converting Annex B to length-prefixed: %v", err)
		return err
	}

	return nil
}

func (mp4 *MP4) Close(config *models.Config) {

	err := mp4.Segment.Encode(mp4.Writer)
	if err != nil {
		panic(err)
	}
	mp4.Segments = append(mp4.Segments, mp4.Segment)
	mp4.Writer.Flush()
	defer mp4.FileWriter.Close()

	// Now we have all the moof and mdat boxes written to the file.
	// We can now generate the ftyp and moov boxes, and replace it with the free box we added earlier (size of 10008 bytes).
	init := mp4ff.NewMP4Init()

	// Create a new ftyp box
	majorBrand := "isom"
	minorVersion := uint32(512)
	compatibleBrands := []string{"iso2", "avc1", "mp41"}
	ftyp := mp4ff.NewFtyp(majorBrand, minorVersion, compatibleBrands)
	init.AddChild(ftyp)
	init.Ftyp.AddCompatibleBrands([]string{"isom", "iso2", "avc1", "mp41"})

	// Create a new moov box
	moov := mp4ff.NewMoovBox()
	init.AddChild(moov)

	// Set the creation time and modification time for the moov box
	videoTimescale := uint32(90000)
	mvhd := &mp4ff.MvhdBox{
		Version:          0,
		Flags:            0,
		CreationTime:     mp4.StartTime,
		ModificationTime: mp4.StartTime,
		Timescale:        videoTimescale, // 90kHz timescale
		Duration:         mp4.TotalDuration,
	}
	init.Moov.AddChild(mvhd)

	// Set the total duration in the moov box
	mvex := mp4ff.NewMvexBox()
	mvex.AddChild(&mp4ff.MehdBox{FragmentDuration: int64(mp4.TotalDuration)})
	init.Moov.AddChild(mvex)

	// Add the video track to the moov box
	// 90kHz timescale for video
	init.AddEmptyTrack(videoTimescale, "video", "und")
	includePS := true
	err = init.Moov.Trak.SetAVCDescriptor("avc1", mp4.SPSNALUs, mp4.PPSNALUs, includePS)
	if err != nil {
		panic(err)
	}
	// Set the total duration in the track header
	init.Moov.Trak.Tkhd.Duration = mp4.TotalDuration
	// Override the HandlerBox, and more specifically the name field with "agent and version"
	init.Moov.Trak.Mdia.Hdlr.Name = "agent " + utils.VERSION

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

	// We will also calculate the SIDX box, which is a segment index box that contains information about the segments in the file.
	// This is useful for seeking in the file, and for streaming the file.
	sidx := &mp4ff.SidxBox{
		Version:                  0,
		Flags:                    0,
		ReferenceID:              0,
		Timescale:                videoTimescale,
		EarliestPresentationTime: 0,
		FirstOffset:              0,
		SidxRefs:                 make([]mp4ff.SidxRef, 0),
	}
	referenceTrak := init.Moov.Trak
	trex, ok := init.Moov.Mvex.GetTrex(referenceTrak.Tkhd.TrackID)
	if !ok {
		// We have an issue.
	}

	segDatas, err := findSegmentData(mp4.Segments, referenceTrak, trex)
	if err != nil {
		// We have an issue.
	}
	fillSidx(sidx, referenceTrak, segDatas, true)

	// Add the SIDX box to the moov box
	init.AddChild(sidx)

	/*
		err = mp4Root.UpdateSidx(addIfNotExists, false)
	*/
	// Get a bit slice writer for the init segment
	// Get a byte buffer of 10008 bytes to write the init segment
	buffer := bytes.NewBuffer(make([]byte, 0))
	init.Encode(buffer)

	// The first 10008 bytes of the file is a free box, so we can read it and replace it with the moov box.
	// The init box might not be 10008 bytes, so we need to read the first 10008 bytes and then replace it with the moov box.
	// while the remaining bytes are for a new free box.
	// Write the init segment at the beginning of the file, replacing the free box
	if _, err := mp4.FileWriter.WriteAt(buffer.Bytes(), 0); err != nil {
		panic(err)
	}

	// Calculate the remaining size for the free box
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
}

type segData struct {
	startPos         uint64
	presentationTime uint64
	baseDecodeTime   uint64
	dur              uint32
	size             uint32
}

func fillSidx(sidx *mp4ff.SidxBox, refTrak *mp4ff.TrakBox, segDatas []segData, nonZeroEPT bool) {
	ept := uint64(0)
	if nonZeroEPT {
		ept = segDatas[0].presentationTime
	}
	sidx.Version = 1
	sidx.Timescale = refTrak.Mdia.Mdhd.Timescale
	sidx.ReferenceID = 1
	sidx.EarliestPresentationTime = ept
	sidx.FirstOffset = 0
	sidx.SidxRefs = make([]mp4ff.SidxRef, 0, len(segDatas))

	for _, segData := range segDatas {
		size := segData.size
		sidx.SidxRefs = append(sidx.SidxRefs, mp4ff.SidxRef{
			ReferencedSize:     size,
			SubSegmentDuration: segData.dur,
			StartsWithSAP:      1,
			SAPType:            1,
		})
	}
}

// findSegmentData returns a slice of segment media data using a reference track.
func findSegmentData(segs []*mp4ff.MediaSegment, refTrak *mp4ff.TrakBox, trex *mp4ff.TrexBox) ([]segData, error) {
	segDatas := make([]segData, 0, len(segs))
	for _, seg := range segs {
		var firstCompositionTimeOffest int64
		dur := uint32(0)
		var baseTime uint64
		for fIdx, frag := range seg.Fragments {
			for _, traf := range frag.Moof.Trafs {
				tfhd := traf.Tfhd
				if tfhd.TrackID == refTrak.Tkhd.TrackID { // Find track that gives sidx time values
					if fIdx == 0 {
						baseTime = traf.Tfdt.BaseMediaDecodeTime()
					}
					for i, trun := range traf.Truns {
						trun.AddSampleDefaultValues(tfhd, trex)
						samples := trun.GetSamples()
						for j, sample := range samples {
							if fIdx == 0 && i == 0 && j == 0 {
								firstCompositionTimeOffest = int64(sample.CompositionTimeOffset)
							}
							dur += sample.Dur
						}
					}
				}
			}
		}
		sd := segData{
			startPos:         seg.StartPos,
			presentationTime: uint64(int64(baseTime) + firstCompositionTimeOffest),
			baseDecodeTime:   baseTime,
			dur:              dur,
			size:             uint32(seg.Size()),
		}
		segDatas = append(segDatas, sd)
	}
	return segDatas, nil
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
