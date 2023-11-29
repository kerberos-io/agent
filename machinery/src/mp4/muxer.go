package mp4

import (
	"fmt"
	"io"
	"time"

	"github.com/kerberos-io/agent/machinery/src/mp4/mp4io"
	"github.com/kerberos-io/agent/machinery/src/packets"
	"github.com/kerberos-io/agent/machinery/src/utils/bits/pio"
)

type Muxer struct {
	w               io.WriteSeeker
	wpos            int64
	streams         []*Stream
	videoCodecIndex int
	AudioCodecIndex int
}

func NewMuxer(w io.WriteSeeker) *Muxer {
	return &Muxer{
		w: w,
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

	stream := &Stream{CodecData: codec}

	stream.sample = &mp4io.SampleTable{
		SampleDesc:   &mp4io.SampleDesc{},
		TimeToSample: &mp4io.TimeToSample{},
		SampleToChunk: &mp4io.SampleToChunk{
			Entries: []mp4io.SampleToChunkEntry{
				{
					FirstChunk:      1,
					SampleDescId:    1,
					SamplesPerChunk: 1,
				},
			},
		},
		SampleSize:  &mp4io.SampleSize{},
		ChunkOffset: &mp4io.ChunkOffset{},
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
	case "H264", "H265":
		stream.sample.SyncSample = &mp4io.SyncSample{}
	}

	stream.timeScale = 10000000
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
		self.sample.SampleDesc.MP4ADesc = &mp4io.MP4ADesc{
			DataRefIdx:       1,
			NumberOfChannels: int16(codec.ChannelLayout().Count()),
			SampleSize:       int16(codec.SampleFormat().BytesPerSample()),
			SampleRate:       float64(codec.SampleRate()),
			Conf: &mp4io.ElemStreamDesc{
				DecConfig: codec.MPEG4AudioConfigBytes(),
			},
		}
		self.trackAtom.Header.Volume = 1
		self.trackAtom.Header.AlternateGroup = 1
		self.trackAtom.Media.Handler = &mp4io.HandlerRefer{
			SubType: [4]byte{'s', 'o', 'u', 'n'},
			Name:    []byte("Sound Handler"),
		}
		self.trackAtom.Media.Info.Sound = &mp4io.SoundMediaInfo{}*/

	} else {
		err = fmt.Errorf("mp4: codec type=%d invalid", self.CodecData.Name)
	}

	return
}

func (self *Muxer) WriteHeader(streams []packets.Stream) (err error) {
	self.streams = []*Stream{}
	for index, stream := range streams {
		if err = self.newStream(stream, index, false); err != nil {
		}
	}

	taghdr := make([]byte, 8)
	pio.PutU32BE(taghdr[4:], uint32(mp4io.MDAT))
	if _, err = self.w.Write(taghdr); err != nil {
		return
	}
	taghdr = nil
	self.wpos += 8

	for _, stream := range self.streams {
		if stream.CodecData.Name == "H264" {
			stream.sample.CompositionOffset = &mp4io.CompositionOffset{}
		}
	}
	return
}

func (self *Muxer) WritePacket(pkt packets.Packet) (err error) {
	stream := self.streams[pkt.Idx]
	switch stream.CodecData.Name {
	case "H264", "AAC":
		if stream.lastpkt != nil {
			if err = stream.writePacket(*stream.lastpkt, pkt.Time-stream.lastpkt.Time); err != nil {
				return
			}
		}
		stream.lastpkt = &pkt
		return
	default:
		return
	}
}

func (self *Muxer) Write(buffer []byte, channel int, time uint32) (err error) {
	return nil
}

func (self *Stream) writePacket(pkt packets.Packet, rawdur time.Duration) (err error) {
	if rawdur < 0 {
		err = fmt.Errorf("mp4: stream#%d time=%v < lasttime=%v", pkt.Idx, pkt.Time, self.lastpkt.Time)
		return
	}

	if _, err = self.muxer.w.Write(pkt.Data); err != nil {
		return
	}

	if pkt.IsKeyFrame && self.sample.SyncSample != nil {
		self.sample.SyncSample.Entries = append(self.sample.SyncSample.Entries, uint32(self.sampleIndex+1))
	}

	duration := uint32(self.timeToTs(rawdur))
	if self.sttsEntry == nil || duration != self.sttsEntry.Duration {
		self.sample.TimeToSample.Entries = append(self.sample.TimeToSample.Entries, mp4io.TimeToSampleEntry{Duration: duration})
		self.sttsEntry = &self.sample.TimeToSample.Entries[len(self.sample.TimeToSample.Entries)-1]
	}
	self.sttsEntry.Count++

	if self.sample.CompositionOffset != nil {
		offset := uint32(self.timeToTs(pkt.CompositionTime))
		if self.cttsEntry == nil || offset != self.cttsEntry.Offset {
			table := self.sample.CompositionOffset
			table.Entries = append(table.Entries, mp4io.CompositionOffsetEntry{Offset: offset})
			self.cttsEntry = &table.Entries[len(table.Entries)-1]
		}
		self.cttsEntry.Count++
	}

	self.duration += int64(duration)
	self.sampleIndex++
	self.sample.ChunkOffset.Entries = append(self.sample.ChunkOffset.Entries, uint32(self.muxer.wpos))
	self.sample.SampleSize.Entries = append(self.sample.SampleSize.Entries, uint32(len(pkt.Data)))

	self.muxer.wpos += int64(len(pkt.Data))
	return
}

func (self *Muxer) WriteTrailer() (err error) {

	for _, stream := range self.streams {
		switch stream.CodecData.Name {
		case "H264", "AAC":
			if stream.lastpkt != nil {
				if err = stream.writePacket(*stream.lastpkt, 0); err != nil {
					//return
				}
				stream.lastpkt = nil
			}
		default:
		}
	}

	moov := &mp4io.Movie{}
	moov.Header = &mp4io.MovieHeader{
		PreferredRate:   1,
		PreferredVolume: 1,
		Matrix:          [9]int32{0x10000, 0, 0, 0, 0x10000, 0, 0, 0, 0x40000000},
		NextTrackId:     2,
	}

	maxDur := time.Duration(0)
	//timeScale := int64(10000)
	timeScale := int64(1000)
	for _, stream := range self.streams {
		switch stream.CodecData.Name {
		case "H264", "AAC":
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
	}
	moov.Header.TimeScale = int32(timeScale)
	moov.Header.Duration = int32(timeToTs(maxDur, timeScale))

	var mdatsize int64
	if mdatsize, err = self.w.Seek(0, 1); err != nil {
		return
	}
	if _, err = self.w.Seek(0, 0); err != nil {
		return
	}
	taghdr := make([]byte, 4)
	pio.PutU32BE(taghdr, uint32(mdatsize))
	if _, err = self.w.Write(taghdr); err != nil {
		return
	}
	taghdr = nil

	if _, err = self.w.Seek(0, 2); err != nil {
		return
	}
	b := make([]byte, moov.Len())
	moov.Marshal(b)
	if _, err = self.w.Write(b); err != nil {
		return
	}
	b = nil
	return
}

func (self *Muxer) WriteTrailerWithPacket(pkt packets.Packet) (err error) {

	for _, stream := range self.streams {
		switch stream.CodecData.Name {
		case "H264", "AAC":
			if stream.lastpkt != nil {
				if err = stream.writePacket(*stream.lastpkt, pkt.Time-stream.lastpkt.Time); err != nil {
					//return
				}
				stream.lastpkt = nil
			}
		default:
		}
	}

	moov := &mp4io.Movie{}
	moov.Header = &mp4io.MovieHeader{
		PreferredRate:   1,
		PreferredVolume: 1,
		Matrix:          [9]int32{0x10000, 0, 0, 0, 0x10000, 0, 0, 0, 0x40000000},
		NextTrackId:     2,
	}

	maxDur := time.Duration(0)
	//timeScale := int64(10000)
	timeScale := int64(1000)
	for _, stream := range self.streams {
		switch stream.CodecData.Name {
		case "H264", "AAC":
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
	}
	moov.Header.TimeScale = int32(timeScale)
	moov.Header.Duration = int32(timeToTs(maxDur, timeScale))

	var mdatsize int64
	if mdatsize, err = self.w.Seek(0, 1); err != nil {
		return
	}
	if _, err = self.w.Seek(0, 0); err != nil {
		return
	}
	taghdr := make([]byte, 4)
	pio.PutU32BE(taghdr, uint32(mdatsize))
	if _, err = self.w.Write(taghdr); err != nil {
		return
	}
	taghdr = nil

	if _, err = self.w.Seek(0, 2); err != nil {
		return
	}
	b := make([]byte, moov.Len())
	moov.Marshal(b)
	if _, err = self.w.Write(b); err != nil {
		return
	}
	b = nil
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
