package capture

import (
	"context"
	"image"

	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/packets"
)

type Capture struct {
	RTSPClient    *Golibrtsp
	RTSPSubClient *Golibrtsp
}

func (c *Capture) SetMainClient(rtspUrl string, withBackChannel bool) *Golibrtsp {
	c.RTSPClient = &Golibrtsp{
		Url:             rtspUrl,
		WithBackChannel: withBackChannel,
	}
	return c.RTSPClient
}

func (c *Capture) SetSubClient(rtspUrl string, withBackChannel bool) *Golibrtsp {
	c.RTSPSubClient = &Golibrtsp{
		Url:             rtspUrl,
		WithBackChannel: withBackChannel,
	}
	return c.RTSPSubClient
}

// RTSPClient is a interface that abstracts the RTSP client implementation.
type RTSPClient interface {
	// Connect to the RTSP server.
	Connect(ctx context.Context) error

	// Start the RTSP client, and start reading packets.
	Start(ctx context.Context, queue *packets.Queue, communication *models.Communication) error

	// Decode a packet into a image.
	DecodePacket(pkt packets.Packet) (image.YCbCr, error)

	// Decode a packet into a image.
	DecodePacketRaw(pkt packets.Packet) (image.Gray, error)

	// Write a packet to the RTSP server.
	WritePacket(pkt packets.Packet) error

	// Close the connection to the RTSP server.
	Close() error

	// Get a list of streams from the RTSP server.
	GetStreams() ([]packets.Stream, error)

	// Get a list of video streams from the RTSP server.
	GetVideoStreams() ([]packets.Stream, error)

	// Get a list of audio streams from the RTSP server.
	GetAudioStreams() ([]packets.Stream, error)
}
