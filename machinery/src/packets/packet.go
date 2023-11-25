package packets

import "time"

// Packet stores compressed audio/video data.
type Packet struct {
	Header          Header // RTP header
	PaddingSize     byte
	IsKeyFrame      bool          // video packet is key frame
	Idx             int8          // stream index in container format
	CompositionTime time.Duration // packet presentation time minus decode time for H264 B-Frame
	Time            time.Duration // packet decode time
	Data            []byte        // packet data

}

type Header struct {
	Version          uint8
	Padding          bool
	Extension        bool
	Marker           bool
	PayloadType      uint8
	SequenceNumber   uint16
	Timestamp        uint32
	SSRC             uint32
	CSRC             []uint32
	ExtensionProfile uint16
	Extensions       []Extension

	// Deprecated: will be removed in a future version.
	PayloadOffset int
}

// Extension RTP Header extension
type Extension struct {
	id      uint8
	payload []byte
}
