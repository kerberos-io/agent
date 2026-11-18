package capture

import (
	"errors"
	"fmt"

	"github.com/pion/rtcp"
)

func (g *Golibrtsp) RequestKeyframe() error {
	g.keyframeMu.Lock()
	defer g.keyframeMu.Unlock()

	if !g.keyframeReady {
		return errors.New("RTSP H264 keyframe requests are unavailable")
	}

	media, ok := g.Client.Stats().Session.Medias[g.VideoH264Media]
	if !ok {
		return errors.New("RTSP video media is not set up")
	}
	video, ok := media.Formats[g.VideoH264Forma]
	if !ok || video.InboundRTPPackets == 0 {
		return errors.New("RTSP video RTP has not arrived yet")
	}

	// Browser and camera SSRCs differ: originate feedback on the RTSP session.
	if err := g.Client.WritePacketRTCP(g.VideoH264Media, &rtcp.PictureLossIndication{
		SenderSSRC: video.LocalSSRC,
		MediaSSRC:  video.RemoteSSRC,
	}); err != nil {
		return fmt.Errorf("request camera keyframe: %w", err)
	}
	return nil
}
