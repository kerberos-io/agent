package packets

import (
	"time"

	"github.com/pion/rtp"
)

// Packet represents an RTP Packet
type Packet struct {
	Packet          *rtp.Packet
	IsAudio         bool   // packet is audio
	IsVideo         bool   // packet is video
	IsKeyFrame      bool   // video packet is key frame
	Idx             int8   // stream index in container format
	Codec           string // codec name
	CompositionTime int64  // packet presentation time minus decode time for H264 B-Frame
	Time            int64  // packet decode time
	TimeLegacy      time.Duration
	Data            []byte // packet data
}
