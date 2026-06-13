package models

import (
	"context"
	"sync/atomic"

	"github.com/kerberos-io/agent/machinery/src/packets"
	"github.com/tevino/abool"
)

type LiveHDSignalingCallbacks struct {
	SendAnswer    func(sessionID string, sdp string) error
	SendCandidate func(sessionID string, candidate string) error
	SendError     func(sessionID string, message string) error
}

type LiveHDHandshake struct {
	Payload   RequestHDStreamPayload
	Signaling *LiveHDSignalingCallbacks
}

// The communication struct that is managing
// all the communication between the different goroutines.
type Communication struct {
	Context               *context.Context
	CancelContext         *context.CancelFunc
	PackageCounter        *atomic.Value
	LastPacketTimer       *atomic.Value
	PackageCounterSub     *atomic.Value
	LastPacketTimerSub    *atomic.Value
	CloudTimestamp        *atomic.Value
	HandleBootstrap       chan string
	HandleStream          chan string
	HandleSubStream       chan string
	HandleMotion          chan MotionDataPartial
	HandleAudio           chan AudioDataPartial
	HandleUpload          chan string
	HandleHeartBeat       chan string
	HandleLiveSD          chan int64
	HandleLiveHDKeepalive chan string
	HandleLiveHDHandshake chan LiveHDHandshake
	HandleLiveHDPeers     chan string
	HandleONVIF           chan OnvifAction
	IsConfiguring         *abool.AtomicBool
	Queue                 *packets.Queue
	SubQueue              *packets.Queue
	Image                 string
	CameraConnected       bool
	MainStreamConnected   bool
	SubStreamConnected    bool
	HasBackChannel        bool
}
