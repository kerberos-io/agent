package models

import (
	"math"
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

type StreamKind uint8

const (
	MainStream StreamKind = iota
	SubStream
)

type StreamRuntimeTelemetry struct {
	Configured        bool
	PackagesProcessed uint64
	FPS               float64
	Width             int64
	Height            int64
	LastPacketAt      int64
}

type streamRuntimeTelemetry struct {
	configured        atomic.Bool
	packagesProcessed atomic.Uint64
	fpsBits           atomic.Uint64
	width             atomic.Int64
	height            atomic.Int64
	lastPacketAt      atomic.Int64
}

type HubRuntimeTelemetry struct {
	Configured                bool
	Connected                 bool
	LastHeartbeatAttemptAt    int64
	LastSuccessfulHeartbeatAt int64
}

type hubRuntimeTelemetry struct {
	configured                atomic.Bool
	connected                 atomic.Bool
	lastHeartbeatAttemptAt    atomic.Int64
	lastSuccessfulHeartbeatAt atomic.Int64
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
	currentRunMu          sync.RWMutex
	currentRun            *AgentRun
	PackageCounter        *atomic.Value
	LastPacketTimer       *atomic.Value
	PackageCounterSub     *atomic.Value
	LastPacketTimerSub    *atomic.Value
	CloudTimestamp        *atomic.Value
	HandleBootstrap       chan string
	HandleStream          chan string
	HandleSubStream       chan string
	HandleAudio           chan AudioDataPartial
	HandleUpload          chan string
	HandleHeartBeat       chan string
	HandleLiveSD          chan int64
	HandleLiveSDHTTP      chan int64
	HandleLiveHDKeepalive chan string
	HandleLiveHDPeers     chan string
	// HandleLiveHLS is the live HLS viewer keepalive. It carries the requested
	// quality tier ("auto"|"high"|"low"; empty => auto) so the producer can switch
	// the live session between the main and sub stream on demand.
	HandleLiveHLS chan string
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
	Image                        string
	CameraConnected              atomic.Bool
	MainStreamConnected          atomic.Bool
	SubStreamConnected           atomic.Bool
	HasBackChannel               atomic.Bool
	mainStreamTelemetry          streamRuntimeTelemetry
	subStreamTelemetry           streamRuntimeTelemetry
	hubTelemetry                 hubRuntimeTelemetry
	recovery                     recoveryTelemetry
}

func (c *Communication) streamTelemetry(stream StreamKind) *streamRuntimeTelemetry {
	switch stream {
	case MainStream:
		return &c.mainStreamTelemetry
	case SubStream:
		return &c.subStreamTelemetry
	default:
		panic("unsupported stream kind")
	}
}

func (c *Communication) SetStreamConfigured(stream StreamKind, configured bool) {
	c.streamTelemetry(stream).configured.Store(configured)
}

func (c *Communication) RecordStreamPackage(stream StreamKind, fps float64, width, height int, at time.Time) {
	telemetry := c.streamTelemetry(stream)
	telemetry.packagesProcessed.Add(1)
	if fps > 0 && !math.IsNaN(fps) && !math.IsInf(fps, 0) {
		telemetry.fpsBits.Store(math.Float64bits(fps))
	}
	if width > 0 {
		telemetry.width.Store(int64(width))
	}
	if height > 0 {
		telemetry.height.Store(int64(height))
	}
	if !at.IsZero() {
		telemetry.lastPacketAt.Store(at.Unix())
	}
}

func (c *Communication) StreamRuntimeTelemetry(stream StreamKind) StreamRuntimeTelemetry {
	telemetry := c.streamTelemetry(stream)
	return StreamRuntimeTelemetry{
		Configured:        telemetry.configured.Load(),
		PackagesProcessed: telemetry.packagesProcessed.Load(),
		FPS:               math.Float64frombits(telemetry.fpsBits.Load()),
		Width:             telemetry.width.Load(),
		Height:            telemetry.height.Load(),
		LastPacketAt:      telemetry.lastPacketAt.Load(),
	}
}

func (c *Communication) SetHubConfigured(configured bool) {
	c.hubTelemetry.configured.Store(configured)
	if !configured {
		c.hubTelemetry.connected.Store(false)
	}
}

func (c *Communication) RecordHubHeartbeatAttempt(at time.Time) {
	if !at.IsZero() {
		c.hubTelemetry.lastHeartbeatAttemptAt.Store(at.Unix())
	}
}

func (c *Communication) RecordHubHeartbeatSuccess(at time.Time) {
	if !at.IsZero() {
		c.hubTelemetry.lastSuccessfulHeartbeatAt.Store(at.Unix())
	}
	c.hubTelemetry.connected.Store(true)
}

func (c *Communication) RecordHubHeartbeatFailure() {
	c.hubTelemetry.connected.Store(false)
}

func (c *Communication) HubRuntimeTelemetry() HubRuntimeTelemetry {
	return HubRuntimeTelemetry{
		Configured:                c.hubTelemetry.configured.Load(),
		Connected:                 c.hubTelemetry.connected.Load(),
		LastHeartbeatAttemptAt:    c.hubTelemetry.lastHeartbeatAttemptAt.Load(),
		LastSuccessfulHeartbeatAt: c.hubTelemetry.lastSuccessfulHeartbeatAt.Load(),
	}
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

func (c *Communication) attachRun(run *AgentRun) error {
	c.currentRunMu.Lock()
	defer c.currentRunMu.Unlock()
	if c.currentRun != nil {
		return ErrAgentRunActive
	}
	c.currentRun = run
	return nil
}

func (c *Communication) detachRun(run *AgentRun) bool {
	c.currentRunMu.Lock()
	defer c.currentRunMu.Unlock()
	if c.currentRun != run {
		return false
	}
	c.currentRun = nil
	return true
}

func (c *Communication) CurrentRun() *AgentRun {
	c.currentRunMu.RLock()
	defer c.currentRunMu.RUnlock()
	return c.currentRun
}

func (c *Communication) MainQueue() *packets.Queue {
	run := c.CurrentRun()
	if run == nil {
		return nil
	}
	return run.MainQueue()
}

func (c *Communication) SubQueue() *packets.Queue {
	run := c.CurrentRun()
	if run == nil {
		return nil
	}
	return run.SubQueue()
}

func (c *Communication) TrySendLiveHDHandshake(handshake LiveHDHandshake) bool {
	run := c.CurrentRun()
	if run == nil || !run.TrySendLiveHDHandshake(handshake) {
		c.recovery.droppedLiveHDHandshakes.Add(1)
		return false
	}
	return true
}

func (c *Communication) PendingLiveHDHandshakes() int {
	run := c.CurrentRun()
	if run == nil {
		return 0
	}
	return run.PendingLiveHDHandshakes()
}

func (c *Communication) TrySendMotion(motion MotionDataPartial) bool {
	run := c.CurrentRun()
	if run == nil || !run.TrySendMotion(motion) {
		c.recovery.droppedMotionEvents.Add(1)
		return false
	}
	return true
}

func (c *Communication) TrySendONVIF(action OnvifAction) bool {
	run := c.CurrentRun()
	if run == nil || !run.TrySendONVIF(action) {
		c.recovery.droppedONVIFActions.Add(1)
		return false
	}
	return true
}
