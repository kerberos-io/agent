package packets

import (
	"time"

	"github.com/pion/rtp"
)

// Packet represents an RTP Packet
type Packet struct {
	Packet      *rtp.Packet
	AccessUnits [][]byte

	// for JOY4 library
	IsKeyFrame      bool          // video packet is key frame
	Idx             int8          // stream index in container format
	CompositionTime time.Duration // packet presentation time minus decode time for H264 B-Frame
	Time            time.Duration // packet decode time
	Data            []byte        // packet data

}
