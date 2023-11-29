package packets

import (
	"time"

	"github.com/pion/rtp"
)

// Packet represents an RTP Packet
type Packet struct {
	// for Gortsplib library
	Packet *rtp.Packet

	// for JOY4 and Gortsplib library
	IsKeyFrame      bool          // video packet is key frame
	Idx             int8          // stream index in container format
	CompositionTime time.Duration // packet presentation time minus decode time for H264 B-Frame
	Time            time.Duration // packet decode time
	Data            []byte        // packet data
}
