package models

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

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

type MoQRecoveryTelemetry struct {
	Reconnects          uint64 `json:"reconnects"`
	LastFrameUnixMillis int64  `json:"lastFrameUnixMillis"`
	LastWriteMillis     int64  `json:"lastWriteMillis"`
}

type RecoveryTelemetry struct {
	MoQHigh                   MoQRecoveryTelemetry `json:"moqHigh"`
	MoQLow                    MoQRecoveryTelemetry `json:"moqLow"`
	MoQWriteTimeouts          uint64               `json:"moqWriteTimeouts"`
	DroppedLiveHDHandshakes   uint64               `json:"droppedLiveHDHandshakes"`
	DroppedMotionEvents       uint64               `json:"droppedMotionEvents"`
	DroppedONVIFActions       uint64               `json:"droppedOnvifActions"`
	WatchdogRestarts          uint64               `json:"watchdogRestarts"`
	WatchdogCooldownSeconds   int64                `json:"watchdogCooldownSeconds"`
	RunWorkerShutdownTimeouts uint64               `json:"runWorkerShutdownTimeouts"`
}

type recoveryTelemetry struct {
	moqHighReconnects          atomic.Uint64
	moqHighLastFrameUnixMillis atomic.Int64
	moqHighLastWriteMillis     atomic.Int64
	moqLowReconnects           atomic.Uint64
	moqLowLastFrameUnixMillis  atomic.Int64
	moqLowLastWriteMillis      atomic.Int64
	moqWriteTimeouts           atomic.Uint64
	droppedLiveHDHandshakes    atomic.Uint64
	droppedMotionEvents        atomic.Uint64
	droppedONVIFActions        atomic.Uint64
	watchdogRestarts           atomic.Uint64
	watchdogCooldownSeconds    atomic.Int64
	runWorkerShutdownTimeouts  atomic.Uint64
}

// The communication struct that is managing
// all the communication between the different goroutines.
type Communication struct {
	runChannelsMu         sync.RWMutex
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
	Queue                        atomic.Pointer[packets.Queue]
	SubQueue                     atomic.Pointer[packets.Queue]
	Image                        string
	CameraConnected              atomic.Bool
	MainStreamConnected          atomic.Bool
	SubStreamConnected           atomic.Bool
	HasBackChannel               atomic.Bool
	recovery                     recoveryTelemetry
}

func (c *Communication) RecordMoQReconnect(quality string) {
	if quality == StreamQualityLow {
		c.recovery.moqLowReconnects.Add(1)
		return
	}
	c.recovery.moqHighReconnects.Add(1)
}

func (c *Communication) RecordMoQWrite(quality string, duration time.Duration, at time.Time) {
	if quality == StreamQualityLow {
		c.recovery.moqLowLastWriteMillis.Store(duration.Milliseconds())
		c.recovery.moqLowLastFrameUnixMillis.Store(at.UnixMilli())
		return
	}
	c.recovery.moqHighLastWriteMillis.Store(duration.Milliseconds())
	c.recovery.moqHighLastFrameUnixMillis.Store(at.UnixMilli())
}

func (c *Communication) RecordMoQWriteTimeout() {
	c.recovery.moqWriteTimeouts.Add(1)
}

func (c *Communication) RecordWatchdogRestart(cooldown time.Duration) {
	c.recovery.watchdogRestarts.Add(1)
	c.SetWatchdogCooldown(cooldown)
}

func (c *Communication) SetWatchdogCooldown(cooldown time.Duration) {
	c.recovery.watchdogCooldownSeconds.Store(int64(cooldown / time.Second))
}

func (c *Communication) RecordRunWorkerShutdownTimeout() {
	c.recovery.runWorkerShutdownTimeouts.Add(1)
}

func (c *Communication) RecoveryTelemetry() RecoveryTelemetry {
	return RecoveryTelemetry{
		MoQHigh: MoQRecoveryTelemetry{
			Reconnects:          c.recovery.moqHighReconnects.Load(),
			LastFrameUnixMillis: c.recovery.moqHighLastFrameUnixMillis.Load(),
			LastWriteMillis:     c.recovery.moqHighLastWriteMillis.Load(),
		},
		MoQLow: MoQRecoveryTelemetry{
			Reconnects:          c.recovery.moqLowReconnects.Load(),
			LastFrameUnixMillis: c.recovery.moqLowLastFrameUnixMillis.Load(),
			LastWriteMillis:     c.recovery.moqLowLastWriteMillis.Load(),
		},
		MoQWriteTimeouts:          c.recovery.moqWriteTimeouts.Load(),
		DroppedLiveHDHandshakes:   c.recovery.droppedLiveHDHandshakes.Load(),
		DroppedMotionEvents:       c.recovery.droppedMotionEvents.Load(),
		DroppedONVIFActions:       c.recovery.droppedONVIFActions.Load(),
		WatchdogRestarts:          c.recovery.watchdogRestarts.Load(),
		WatchdogCooldownSeconds:   c.recovery.watchdogCooldownSeconds.Load(),
		RunWorkerShutdownTimeouts: c.recovery.runWorkerShutdownTimeouts.Load(),
	}
}

func (c *Communication) SetRunChannels(handshakes chan LiveHDHandshake, motion chan MotionDataPartial, onvif chan OnvifAction) {
	c.runChannelsMu.Lock()
	c.HandleLiveHDHandshake = handshakes
	c.HandleMotion = motion
	c.HandleONVIF = onvif
	c.runChannelsMu.Unlock()
}

func (c *Communication) CloseRunChannels() {
	c.runChannelsMu.Lock()
	handshakes := c.HandleLiveHDHandshake
	motion := c.HandleMotion
	onvif := c.HandleONVIF
	c.HandleLiveHDHandshake = nil
	c.HandleMotion = nil
	c.HandleONVIF = nil
	if handshakes != nil {
		close(handshakes)
	}
	if motion != nil {
		close(motion)
	}
	if onvif != nil {
		close(onvif)
	}
	c.runChannelsMu.Unlock()
}

func (c *Communication) TrySendLiveHDHandshake(handshake LiveHDHandshake) bool {
	c.runChannelsMu.RLock()
	defer c.runChannelsMu.RUnlock()
	if c.HandleLiveHDHandshake == nil {
		c.recovery.droppedLiveHDHandshakes.Add(1)
		return false
	}
	select {
	case c.HandleLiveHDHandshake <- handshake:
		return true
	default:
		c.recovery.droppedLiveHDHandshakes.Add(1)
		return false
	}
}

func (c *Communication) PendingLiveHDHandshakes() int {
	c.runChannelsMu.RLock()
	defer c.runChannelsMu.RUnlock()
	return len(c.HandleLiveHDHandshake)
}

func (c *Communication) TrySendMotion(motion MotionDataPartial) bool {
	c.runChannelsMu.RLock()
	defer c.runChannelsMu.RUnlock()
	if c.HandleMotion == nil {
		c.recovery.droppedMotionEvents.Add(1)
		return false
	}
	select {
	case c.HandleMotion <- motion:
		return true
	default:
		c.recovery.droppedMotionEvents.Add(1)
		return false
	}
}

func (c *Communication) TrySendONVIF(action OnvifAction) bool {
	c.runChannelsMu.RLock()
	defer c.runChannelsMu.RUnlock()
	if c.HandleONVIF == nil {
		c.recovery.droppedONVIFActions.Add(1)
		return false
	}
	select {
	case c.HandleONVIF <- action:
		return true
	default:
		c.recovery.droppedONVIFActions.Add(1)
		return false
	}
}
