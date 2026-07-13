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
	HandleLiveSDHTTP      chan int64
	HandleLiveHDKeepalive chan string
	HandleLiveHDHandshake chan LiveHDHandshake
	HandleLiveHDPeers     chan string
	// HandleLiveHLS is the live HLS viewer keepalive. It carries the requested
	// quality tier ("auto"|"high"|"low"; empty => auto) so the producer can switch
	// the live session between the main and sub stream on demand.
	HandleLiveHLS chan string
	HandleONVIF   chan OnvifAction
	IsConfiguring *abool.AtomicBool
	// IsRecordingManual is set while a viewer has requested a manual recording
	// from the live view (the record button). While set, the motion-based
	// recorder keeps recording (it does not auto-close on the post-recording
	// timeout) until the viewer stops it again. It is independent of motion
	// detection so it also works when nothing is moving.
	IsRecordingManual *abool.AtomicBool
	// RecordingManualHeartbeat holds the unix-milliseconds timestamp of the last
	// heartbeat received from the live view while a manual recording is active.
	// The frontend re-sends the record command every few seconds while the user
	// stays on the page; if the heartbeats stop (the viewer closed the tab, went
	// idle or lost connectivity) the recorder auto-stops the manual recording so
	// it can't record forever when the "stop" message never arrives.
	RecordingManualHeartbeat *atomic.Int64
	// RecordingManualStart holds the unix-milliseconds timestamp at which the
	// current manual recording started. It bounds a manual recording to a maximum
	// duration (see capture.manualRecordingMaxDuration) so a forgotten record
	// button can't record indefinitely even while the viewer stays active.
	RecordingManualStart *atomic.Int64
	// RecordingManualHeartbeatSeen is set once the current manual recording has
	// received at least one heartbeat, i.e. the viewer proved it supports
	// heartbeating. Only then does the recorder enforce the heartbeat timeout; a
	// viewer that starts a recording but never heartbeats (an older frontend)
	// still records up to the max-duration cap instead of being cut off early.
	RecordingManualHeartbeatSeen *abool.AtomicBool
	Queue                        *packets.Queue
	SubQueue                     *packets.Queue
	Image                        string
	CameraConnected              bool
	MainStreamConnected          bool
	SubStreamConnected           bool
	HasBackChannel               bool
}
