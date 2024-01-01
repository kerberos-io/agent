package packets

import (
	"time"

	"github.com/pion/rtp"
)

// Packet represents an RTP Packet
type Packet struct {
	Packet          *rtp.Packet
	IsAudio         bool          // packet is audio
	IsVideo         bool          // packet is video
	IsKeyFrame      bool          // video packet is key frame
	Idx             int8          // stream index in container format
	Codec           string        // codec name
	CompositionTime time.Duration // packet presentation time minus decode time for H264 B-Frame
	Time            time.Duration // packet decode time
	Data            []byte        // packet data
}
