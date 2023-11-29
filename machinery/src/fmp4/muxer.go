package fmp4

import (
	"bufio"
	"fmt"
	"io"
	"time"

	"github.com/kerberos-io/agent/machinery/src/fmp4/mp4io"
	"github.com/kerberos-io/agent/machinery/src/packets"
	"github.com/kerberos-io/agent/machinery/src/utils/bits/pio"
)

type Muxer struct {
	w               io.WriteSeeker
	bufw            *bufio.Writer
	wpos            int64
	streams         []*Stream
	videoCodecIndex int
	AudioCodecIndex int

	moof_seqnum uint32
	// Streams must start with Keyframes / IDRs
	// A keyframe is a complete sample that contains all information to produce a single image.
	// All other samples are deltas w.r.t to the last keyframe that's why MP4's must
	// always start with a keyframe because any other type of frame will have not point of reference.
	// It does mean we lose some data but it was useless anyways.
	// This is on the muxer & not on an individual stream to prevent audio (pre first keyframe) of making
	// it into our MP4 essentially delaying the audio by a few seconds perhaps (depends on keyframe interval).
	gotFirstKeyframe bool
}

func NewMuxer(w io.WriteSeeker) *Muxer {
	return &Muxer{
		w:    w,
		bufw: bufio.NewWriterSize(w, pio.RecommendBufioSize),
	}
}

func (self *Muxer) newStream(codec packets.Stream, index int, withoutAudio bool) (err error) {

	switch codec.Name {
	case "H264":
		self.videoCodecIndex = index
	case "AAC":
	default:
		self.AudioCodecIndex = index
		if withoutAudio {
			return
		}
	}

	stream := &Stream{
		CodecData: codec,
		Idx:       index,
	}

	stream.sample = &mp4io.SampleTable{
		SampleDesc:    &mp4io.SampleDesc{},
		TimeToSample:  &mp4io.TimeToSample{},
		SampleToChunk: &mp4io.SampleToChunk{},
		SampleSize:    &mp4io.SampleSize{},
		ChunkOffset:   &mp4io.ChunkOffset{},
	}

	stream.trackAtom = &mp4io.Track{
		Header: &mp4io.TrackHeader{
			TrackId:  int32(len(self.streams) + 1),
			Flags:    0x0003, // Track enabled | Track in movie
			Duration: 0,      // fill later
			Matrix:   [9]int32{0x10000, 0, 0, 0, 0x10000, 0, 0, 0, 0x40000000},
		},
		Media: &mp4io.Media{
			Header: &mp4io.MediaHeader{
				TimeScale: 0, // fill later
				Duration:  0, // fill later
				Language:  21956,
			},
			Info: &mp4io.MediaInfo{
				Sample: stream.sample,
				Data: &mp4io.DataInfo{
					Refer: &mp4io.DataRefer{
						Url: &mp4io.DataReferUrl{
							Flags: 0x000001, // Self reference
						},
					},
				},
			},
		},
	}

	switch codec.Name {
	case "H264":
		stream.sample.SyncSample = &mp4io.SyncSample{}
	}

	stream.timeScale = 12288 // 90000 //
	stream.muxer = self
	self.streams = append(self.streams, stream)

	return
}

func (self *Stream) fillTrackAtom() (err error) {
	self.trackAtom.Media.Header.TimeScale = int32(self.timeScale)
	self.trackAtom.Media.Header.Duration = int32(self.duration)

	if self.CodecData.Name == "H264" {

		codec := self.CodecData
		width, height := codec.Width, codec.Height
		decoderData := []byte{}

		recordinfo := AVCDecoderConfRecord{}
		if len(self.CodecData.SPS) > 0 {
			recordinfo.AVCProfileIndication = self.CodecData.SPS[1]
			recordinfo.ProfileCompatibility = self.CodecData.SPS[2]
			recordinfo.AVCLevelIndication = self.CodecData.SPS[3]
			recordinfo.SPS = [][]byte{self.CodecData.SPS}

		}
		if len(self.CodecData.PPS) > 0 {
			recordinfo.PPS = [][]byte{self.CodecData.PPS}
		}
		recordinfo.LengthSizeMinusOne = 3 // check...

		buf := make([]byte, recordinfo.Len())
		recordinfo.Marshal(buf)

		self.sample.SampleDesc.AVC1Desc = &mp4io.AVC1Desc{
			DataRefIdx:           1,
			HorizontalResolution: 72,
			VorizontalResolution: 72,
			Width:                int16(width),
			Height:               int16(height),
			FrameCount:           1,
			Depth:                24,
			ColorTableId:         -1,
			Conf:                 &mp4io.AVC1Conf{Data: decoderData},
		}
		self.trackAtom.Media.Handler = &mp4io.HandlerRefer{
			SubType: [4]byte{'v', 'i', 'd', 'e'},
			Name:    []byte("Video Media Handler"),
		}
		self.trackAtom.Media.Info.Video = &mp4io.VideoMediaInfo{
			Flags: 0x000001,
		}
		self.trackAtom.Header.TrackWidth = float64(width)
		self.trackAtom.Header.TrackHeight = float64(height)

	} else if self.CodecData.Name == "AAC" {
		/*codec := self.CodecData.(aacparser.CodecData)
		audioConfig := codec.MPEG4AudioConfigBytes()
		self.sample.SampleDesc.MP4ADesc = &mp4io.MP4ADesc{
			DataRefIdx:       2,
			NumberOfChannels: int16(codec.ChannelLayout().Count()),
			SampleSize:       int16(codec.SampleFormat().BytesPerSample()),
			SampleRate:       float64(codec.SampleRate()),
			Conf: &mp4io.ElemStreamDesc{
				DecConfig: audioConfig,
			},
		}
		self.trackAtom.Header.Volume = 1
		self.trackAtom.Header.AlternateGroup = 1
		self.trackAtom.Media.Handler = &mp4io.HandlerRefer{
			SubType: [4]byte{'s', 'o', 'u', 'n'},
			Name:    []byte{'S', 'o', 'u', 'n', 'd', 'H', 'a', 'n', 'd', 'l', 'e', 'r', 0},
		}
		self.trackAtom.Media.Info.Sound = &mp4io.SoundMediaInfo{}*/

	} else {
		err = fmt.Errorf("mp4: codec type=%d invalid", self.CodecData.Name)
	}

	return
}

func (self *Muxer) WriteHeader(streams []packets.Stream) (err error) {
	self.streams = []*Stream{}
	for i, stream := range streams {
		if err = self.newStream(stream, i, false); err != nil {
			// no need to stop the recording if a codec doesnt match, still try to...
		}
	}
	/*
		https://www.w3.org/2013/12/byte-stream-format-registry/isobmff-byte-stream-format.html#h2_iso-init-segments
		The user agent must run the end of stream algorithm with the error parameter set to "decode" if any of the following conditions are met:
			- A File Type Box contains a major_brand or compatible_brand that the user agent does not support.
			- A box or field in the Movie Header Box is encountered that violates the requirements mandated by the major_brand or one of the compatible_brands in the File Type Box.
			- The tracks in the Movie Header Box contain samples (i.e. the entry_count in the stts, stsc or stco boxes are not set to zero).
			- A Movie Extends (mvex) box is not contained in the Movie (moov) box to indicate that Movie Fragments are to be expected.
	*/

	moov := &mp4io.Movie{}
	moov.Header = &mp4io.MovieHeader{
		PreferredRate:     1,
		PreferredVolume:   1,
		Matrix:            [9]int32{0x10000, 0, 0, 0, 0x10000, 0, 0, 0, 0x40000000},
		NextTrackId:       int32(len(self.streams)), // ffmpeg uses the last track id as the next track id, makes no sense
		PreviewTime:       time.Date(1904, time.January, 1, 0, 0, 0, 0, time.UTC),
		PreviewDuration:   time.Date(1904, time.January, 1, 0, 0, 0, 0, time.UTC),
		PosterTime:        time.Date(1904, time.January, 1, 0, 0, 0, 0, time.UTC),
		SelectionTime:     time.Date(1904, time.January, 1, 0, 0, 0, 0, time.UTC),
		SelectionDuration: time.Date(1904, time.January, 1, 0, 0, 0, 0, time.UTC),
		CurrentTime:       time.Date(1904, time.January, 1, 0, 0, 0, 0, time.UTC),
	}

	// Movie Extend MVEX is required for fragmented MP4s
	trackExtends := make([]*mp4io.TrackExtend, 0)
	for _, stream := range self.streams {
		// Add an extension for every available track along with their track ids.
		ext := &mp4io.TrackExtend{
			TrackId:              uint32(stream.Idx),
			DefaultSampleDescIdx: uint32(1),
		}
		trackExtends = append(trackExtends, ext)
	}
	moov.MovieExtend = &mp4io.MovieExtend{
		Tracks: trackExtends,
	}

	// TODO(atom): write a parser of the User Data Box (udta)

	maxDur := time.Duration(0)
	timeScale := int64(10000)
	for _, stream := range self.streams {
		if err = stream.fillTrackAtom(); err != nil {
			return
		}
		dur := stream.tsToTime(stream.duration)
		stream.trackAtom.Header.Duration = int32(timeToTs(dur, timeScale))
		if dur > maxDur {
			maxDur = dur
		}
		moov.Tracks = append(moov.Tracks, stream.trackAtom)
	}
	moov.Header.TimeScale = int32(timeScale)
	moov.Header.Duration = int32(timeToTs(maxDur, timeScale))

	b := make([]byte, moov.Len())
	moov.Marshal(b)
	if _, err = self.w.Write(b); err != nil {
		return
	}

	return
}

func (self *Stream) BuildTrackFragmentWithoutOffset() (trackFragment *mp4io.TrackFrag, err error) {
	// new duration
	newDts := self.dts

	// Create TrackFragRunEntries
	trackfragentries := make([]mp4io.TrackFragRunEntry, 0) // https://ffmpeg.org/pipermail/ffmpeg-devel/2014-November/164898.html
	// Loop over all the samples and build the Track Fragment Entries.
	// Each sample gets its own entry which essentially captures the duration of the sample
	// and the location within the MDAT relative by the size of the sample.
	for _, pkt := range self.pkts {

		// Calculate the duration of the frame, if no previous frames were recorded then
		// invent a timestamp (1ms) for it to make sure it's not 0.
		var duration time.Duration
		if self.lastpkt != nil {
			duration = pkt.Time - self.lastpkt.Time
		}

		// Increment the decode timestamp for the next Track Fragment Decode Time (TFDT)
		// Essentially it is the combination of the durations of the samples.
		newDts += self.timeToTs(duration)

		/*
			if duration == 0 {
				duration = 40 * time.Millisecond
			}*/

		// Audio tends to build very predictable packets due to its sampling rate
		// A possible optimization would be to rely on the default flags instead.
		// This requires looping over all packets and verifying the default size & duration.const
		// Saves a few bytes for each trun entry and could be looked into.const
		// Current behavior is to explicitly write all of the entries their size & duration.
		entry := mp4io.TrackFragRunEntry{
			Duration: uint32(self.timeToTs(duration)), // The timescaled duration e.g 2999
			Size:     uint32(len(pkt.Data)),           // The length of the sample in bytes e.g 51677
			Flags:    uint32(33554432),
			Cts:      uint32(self.timeToTs(pkt.CompositionTime)), // Composition timestamp is typically for B-frames, which are not used in RTSP
		}
		trackfragentries = append(trackfragentries, entry)
		self.lastpkt = pkt
	}

	// Build the Track Fragment
	DefaultSampleFlags := uint32(0)
	if self.CodecData.Name == "H264" {
		DefaultSampleFlags = 16842752
	} else {
		// audio
		DefaultSampleFlags = 33554432
	}

	// If no fragment entries are available, then just set the durations to 512
	// TODO: demuxer bug for B-frames has the same dts
	DefaultDuration := uint32(512)

	// Set the track frag flags such that they include the flag for CTS
	trackFragRunFlags := uint32(mp4io.TRUN_DATA_OFFSET | mp4io.TRUN_FIRST_SAMPLE_FLAGS | mp4io.TRUN_SAMPLE_SIZE | mp4io.TRUN_SAMPLE_DURATION)
	if self.hasBFrames { // TODO: add check if video track
		//trackFragRunFlags = trackFragRunFlags | mp4io.TRUN_SAMPLE_CTS
		trackFragRunFlags = uint32(mp4io.TRUN_DATA_OFFSET | mp4io.TRUN_FIRST_SAMPLE_FLAGS | mp4io.TRUN_SAMPLE_SIZE | mp4io.TRUN_SAMPLE_CTS)
		// TODO: in ffmpeg this is 33554432 for video track & none if audio
	}

	FirstSampleFlags := uint32(mp4io.TRUN_SAMPLE_SIZE | mp4io.TRUN_SAMPLE_DURATION) // mp4io.TRUN_DATA_OFFSET | mp4io.TRUN_FIRST_SAMPLE_FLAGS |
	// The first packet is a b-frame so set the first sample flags to have a CTS.
	if len(self.pkts) > 0 && self.pkts[0].CompositionTime > 0 {
		FirstSampleFlags = uint32(mp4io.TRUN_SAMPLE_SIZE | mp4io.TRUN_SAMPLE_CTS)
	}

	trackFragment = &mp4io.TrackFrag{
		Header: &mp4io.TrackFragHeader{
			Version:         uint8(0),
			Flags:           uint32(mp4io.TFHD_DEFAULT_FLAGS | mp4io.TFHD_DEFAULT_DURATION | mp4io.TFHD_DEFAULT_BASE_IS_MOOF), // uint32(131128),
			TrackId:         uint32(self.Idx),
			DefaultDuration: DefaultDuration,
			DefaultFlags:    DefaultSampleFlags, // TODO: fix to real flags
		},
		DecodeTime: &mp4io.TrackFragDecodeTime{
			Version:    uint8(1), // Decides whether 1 = 64bit, 0 = 32bit timestamp
			Flags:      uint32(0),
			DecodeTime: uint64(self.dts), // Decode timestamp timescaled
		},
		Run: &mp4io.TrackFragRun{
			Version:          uint8(0),
			Flags:            trackFragRunFlags, // The flags if 0 then no DataOffset & no FirstSampleFlags
			DataOffset:       uint32(368),       // NOTE: this is rewritten later
			FirstSampleFlags: FirstSampleFlags,
			Entries:          trackfragentries,
		},
	}

	// Set the next dts
	newDts += self.timeToTs(1 * time.Millisecond)
	self.dts = newDts

	// Reset hasBFrames
	self.hasBFrames = false

	return
}

func (self *Muxer) flushMoof() (err error) {

	// Build the Track Frags
	trackFragments := make([]*mp4io.TrackFrag, 0)
	for _, stream := range self.streams {
		// Build the Track Frag for this stream
		var trackFragment *mp4io.TrackFrag
		trackFragment, err = stream.BuildTrackFragmentWithoutOffset()
		if err != nil {
			return
		}
		trackFragments = append(trackFragments, trackFragment)
	}

	// Defer the clearing of the packets, we'll need them later in this function to
	// write the MDAT contents & calculate its size.
	defer func() {
		for _, stream := range self.streams {
			stream.pkts = make([]*packets.Packet, 0)
		}
	}()

	moof := &mp4io.MovieFrag{
		Header: &mp4io.MovieFragHeader{
			Version: uint8(0),
			Flags:   uint32(0),
			Seqnum:  self.moof_seqnum,
		},
		Tracks: trackFragments,
	}

	// Fix the dataoffsets of the track run
	nextDataOffset := uint32(moof.Len() + 8)
	for _, track := range moof.Tracks {
		track.Run.DataOffset = nextDataOffset
		for _, entry := range track.Run.Entries {
			nextDataOffset += entry.Size
		}
	}

	// Write the MOOF
	b := make([]byte, moof.Len())
	moof.Marshal(b)
	if _, err = self.w.Write(b); err != nil {
		return
	}
	b = nil

	// Write the MDAT size
	mdatsize := uint32(8) // skip itself
	for _, fragment := range trackFragments {
		for _, entry := range fragment.Run.Entries {
			mdatsize += entry.Size
		}
	}

	taghdr := make([]byte, 4)
	pio.PutU32BE(taghdr, mdatsize)
	if _, err = self.w.Write(taghdr); err != nil {
		return
	}
	taghdr = nil

	// Write the MDAT header
	taghdr = make([]byte, 4)
	pio.PutU32BE(taghdr, uint32(mp4io.MDAT))
	if _, err = self.w.Write(taghdr); err != nil {
		return
	}
	taghdr = nil

	// Write the MDAT contents
	for _, stream := range self.streams {
		for _, pkt := range stream.pkts {
			if _, err = self.w.Write(pkt.Data); err != nil {
				return
			}
		}
	}

	// Increment the SeqNum
	self.moof_seqnum++

	return
}

func (self *Muxer) Write(buffer []byte, channel int, time uint32) (err error) {
	return nil
}

func (self *Muxer) WritePacket(pkt packets.Packet) (err error) {
	// Check if pkt.Idx is a valid stream
	if len(self.streams) < int(pkt.Idx+1) {
		return
	}
	stream := self.streams[pkt.Idx]

	// Wait until we have a video packet & it's a keyframe
	if pkt.IsKeyFrame && !self.gotFirstKeyframe && stream.CodecData.IsVideo {
		// First keyframe found, we can start processing
		self.gotFirstKeyframe = true
	} else if !self.gotFirstKeyframe {
		// Skip all packets until keyframe first
		return
	} else if pkt.IsKeyFrame {
		// At this point, we have a keyframe and had one before.
		self.flushMoof()
	}

	if err = stream.writePacket(pkt); err != nil {
		return
	}

	return
}

func (self *Stream) writePacket(pkt packets.Packet /*, rawdur time.Duration*/) (err error) {
	self.pkts = append(self.pkts, &pkt)
	// Optimization: set the has B Frames boolean to indicate that there are B-Frames
	// that require the TrackFragRun will require the CTS flags.
	self.hasBFrames = self.hasBFrames || pkt.CompositionTime > 0
	return
}

func (self *Muxer) WriteTrailer() (err error) {
	self.bufw = nil
	self.streams = nil
	return
}

func (self *Muxer) WriteTrailerWithPacket(pkt packets.Packet) (err error) {
	// Check if pkt.Idx is a valid stream
	if len(self.streams) < int(pkt.Idx+1) {
		return
	}
	stream := self.streams[pkt.Idx]

	// Wait until we have a video packet & it's a keyframe
	if pkt.IsKeyFrame && !self.gotFirstKeyframe && stream.CodecData.IsVideo {
		// First keyframe found, we can start processing
		self.gotFirstKeyframe = true
	} else if !self.gotFirstKeyframe {
		// Skip all packets until keyframe first
		return
	} else if pkt.IsKeyFrame {
		// At this point, we have a keyframe and had one before.
		self.flushMoof()
	}

	if err = stream.writePacket(pkt); err != nil {
		return
	}

	self.bufw = nil
	self.streams = nil

	return
}

func (self *Muxer) Close() (err error) {
	for _, stream := range self.streams {
		stream.muxer = nil
		stream.trackAtom = nil
		stream.sample = nil
		stream.lastpkt = nil
		stream = nil
	}
	self.streams = nil
	return
}

type AVCDecoderConfRecord struct {
	AVCProfileIndication uint8
	ProfileCompatibility uint8
	AVCLevelIndication   uint8
	LengthSizeMinusOne   uint8
	SPS                  [][]byte
	PPS                  [][]byte
}

func (self AVCDecoderConfRecord) Len() (n int) {
	n = 7
	for _, sps := range self.SPS {
		n += 2 + len(sps)
	}
	for _, pps := range self.PPS {
		n += 2 + len(pps)
	}
	return
}

func (self AVCDecoderConfRecord) Marshal(b []byte) (n int) {
	b[0] = 1
	b[1] = self.AVCProfileIndication
	b[2] = self.ProfileCompatibility
	b[3] = self.AVCLevelIndication
	b[4] = self.LengthSizeMinusOne | 0xfc
	b[5] = uint8(len(self.SPS)) | 0xe0
	n += 6

	for _, sps := range self.SPS {
		pio.PutU16BE(b[n:], uint16(len(sps)))
		n += 2
		copy(b[n:], sps)
		n += len(sps)
	}

	b[n] = uint8(len(self.PPS))
	n++

	for _, pps := range self.PPS {
		pio.PutU16BE(b[n:], uint16(len(pps)))
		n += 2
		copy(b[n:], pps)
		n += len(pps)
	}

	return
}
