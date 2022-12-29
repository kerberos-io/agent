package models

import (
	"sync"
	"sync/atomic"

	"github.com/kerberos-io/joy4/av/pubsub"
	"github.com/kerberos-io/joy4/cgo/ffmpeg"
	"github.com/tevino/abool"
)

// The communication struct that is managing
// all the communication between the different goroutines.
type Communication struct {
	PackageCounter        *atomic.Value
	LastPacketTimer       *atomic.Value
	CloudTimestamp        *atomic.Value
	HandleBootstrap       chan string
	HandleStream          chan string
	HandleSubStream       chan string
	HandleMotion          chan MotionDataPartial
	HandleUpload          chan string
	HandleHeartBeat       chan string
	HandleLiveSD          chan int64
	HandleLiveHDKeepalive chan string
	HandleLiveHDHandshake chan SDPPayload
	HandleLiveHDPeers     chan string
	HandleONVIF           chan OnvifAction
	IsConfiguring         *abool.AtomicBool
	Queue                 *pubsub.Queue
	DecoderMutex          *sync.Mutex
	Decoder               *ffmpeg.VideoDecoder
	SubDecoder            *ffmpeg.VideoDecoder
}
