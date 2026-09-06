package models

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kerberos-io/agent/machinery/src/lifecycle"
	"github.com/kerberos-io/agent/machinery/src/packets"
	log "github.com/sirupsen/logrus"
)

var (
	ErrAgentRunActive  = errors.New("another agent run is active")
	ErrAgentRunStarted = errors.New("agent run already started")
	ErrAgentRunStopped = errors.New("agent run is stopping")
	nextAgentRunID     atomic.Uint64
)

type AgentRunClient interface {
	Close(context.Context) error
}

type AgentRunResourceError struct {
	Resource string
	Err      error
}

func (e AgentRunResourceError) Error() string {
	return fmt.Sprintf("close %s: %v", e.Resource, e.Err)
}

func (e AgentRunResourceError) Unwrap() error {
	return e.Err
}

type AgentRunShutdownReport struct {
	lifecycle.ShutdownReport
	ResourceErrors      []AgentRunResourceError
	UploadStopDelivered bool
	StreamStopDelivered bool
}

type AgentRun struct {
	id            uint64
	ctx           context.Context
	cancel        context.CancelCauseFunc
	supervisor    *lifecycle.Supervisor
	communication *Communication
	stopUpload    bool

	stateMu   sync.Mutex
	activated bool
	stopping  bool

	resourcesMu       sync.RWMutex
	mainClient        AgentRunClient
	subClient         AgentRunClient
	backchannelClient AgentRunClient
	mainQueue         *packets.Queue
	subQueue          *packets.Queue
	releaseClients    func()

	channelsMu       sync.RWMutex
	channelsClosed   bool
	liveHDHandshakes chan LiveHDHandshake
	motionEvents     chan MotionDataPartial
	onvifActions     chan OnvifAction

	shutdownOnce   sync.Once
	shutdownReport AgentRunShutdownReport
}

func NewAgentRun(parent context.Context, communication *Communication, stopUpload bool) *AgentRun {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancelCause(parent)
	run := &AgentRun{
		id:               nextAgentRunID.Add(1),
		ctx:              ctx,
		cancel:           cancel,
		supervisor:       lifecycle.NewSupervisor(ctx),
		communication:    communication,
		stopUpload:       stopUpload,
		liveHDHandshakes: make(chan LiveHDHandshake, 100),
		motionEvents:     make(chan MotionDataPartial, 10),
		onvifActions:     make(chan OnvifAction, 10),
	}
	log.WithFields(log.Fields{
		"component":   "agent_run",
		"event":       "run_created",
		"run_id":      run.id,
		"stop_upload": stopUpload,
	}).Debug("Agent run created")
	return run
}

func (r *AgentRun) ID() uint64 {
	return r.id
}

func (r *AgentRun) Context() context.Context {
	return r.ctx
}

func (r *AgentRun) Go(name string, policy lifecycle.TaskPolicy, task lifecycle.TaskFunc) error {
	if task == nil {
		return r.supervisor.Go(name, policy, task)
	}

	fields := log.Fields{
		"component":    "agent_run",
		"event":        "task_lifecycle",
		"long_running": policy.LongRunning,
		"required":     policy.Required,
		"run_id":       r.id,
		"task":         name,
	}
	wrappedTask := func(ctx context.Context) error {
		startedAt := time.Now()
		log.WithFields(fields).Debug("Agent task started")
		err := task(ctx)
		entry := log.WithFields(fields).
			WithField("duration_ms", time.Since(startedAt).Milliseconds())
		if err != nil {
			entry = entry.WithError(err)
			if ctx.Err() == nil {
				entry.Warn("Agent task stopped with an error")
			} else {
				entry.Debug("Agent task stopped during shutdown")
			}
		} else {
			entry.Debug("Agent task stopped")
		}
		return err
	}

	if err := r.supervisor.Go(name, policy, wrappedTask); err != nil {
		log.WithError(err).WithFields(fields).Error("Failed to register Agent task")
		return err
	}
	log.WithFields(fields).Debug("Agent task registered")
	return nil
}

func (r *AgentRun) Seal() {
	r.supervisor.Seal()
	log.WithFields(log.Fields{
		"component":  "agent_run",
		"event":      "run_sealed",
		"run_id":     r.id,
		"task_count": len(r.supervisor.Snapshot()),
	}).Debug("Agent run sealed")
}

func (r *AgentRun) Failures() <-chan lifecycle.Failure {
	return r.supervisor.Failures()
}

func (r *AgentRun) Snapshot() []lifecycle.TaskSnapshot {
	return r.supervisor.Snapshot()
}

func (r *AgentRun) Wait(ctx context.Context) lifecycle.ShutdownReport {
	return r.supervisor.Wait(ctx)
}

func (r *AgentRun) Activate() error {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	if r.stopping {
		return ErrAgentRunStopped
	}
	if r.ctx.Err() != nil {
		return ErrAgentRunStopped
	}
	if r.activated {
		return ErrAgentRunStarted
	}
	if r.communication == nil {
		return errors.New("agent run communication is required")
	}
	if err := r.communication.attachRun(r); err != nil {
		return err
	}
	r.activated = true
	log.WithFields(log.Fields{
		"component": "agent_run",
		"event":     "run_activated",
		"run_id":    r.id,
	}).Info("Agent run activated")
	return nil
}

func (r *AgentRun) SetMainClient(client AgentRunClient) {
	r.resourcesMu.Lock()
	r.mainClient = client
	r.resourcesMu.Unlock()
}

func (r *AgentRun) SetSubClient(client AgentRunClient) {
	r.resourcesMu.Lock()
	r.subClient = client
	r.resourcesMu.Unlock()
}

func (r *AgentRun) SetBackchannelClient(client AgentRunClient) {
	r.resourcesMu.Lock()
	r.backchannelClient = client
	r.resourcesMu.Unlock()
}

func (r *AgentRun) SetQueues(mainQueue, subQueue *packets.Queue) {
	r.resourcesMu.Lock()
	r.mainQueue = mainQueue
	r.subQueue = subQueue
	r.resourcesMu.Unlock()
}

func (r *AgentRun) SetMainQueue(queue *packets.Queue) {
	r.resourcesMu.Lock()
	r.mainQueue = queue
	r.resourcesMu.Unlock()
}

func (r *AgentRun) SetSubQueue(queue *packets.Queue) {
	r.resourcesMu.Lock()
	r.subQueue = queue
	r.resourcesMu.Unlock()
}

func (r *AgentRun) SetClientRelease(release func()) {
	r.resourcesMu.Lock()
	r.releaseClients = release
	r.resourcesMu.Unlock()
}

func (r *AgentRun) MainQueue() *packets.Queue {
	if r.isStopping() {
		return nil
	}
	r.resourcesMu.RLock()
	defer r.resourcesMu.RUnlock()
	return r.mainQueue
}

func (r *AgentRun) SubQueue() *packets.Queue {
	if r.isStopping() {
		return nil
	}
	r.resourcesMu.RLock()
	defer r.resourcesMu.RUnlock()
	return r.subQueue
}

func (r *AgentRun) LiveHDHandshakes() <-chan LiveHDHandshake {
	return r.liveHDHandshakes
}

func (r *AgentRun) MotionEvents() <-chan MotionDataPartial {
	return r.motionEvents
}

func (r *AgentRun) ONVIFActions() <-chan OnvifAction {
	return r.onvifActions
}

func (r *AgentRun) TrySendLiveHDHandshake(handshake LiveHDHandshake) bool {
	if r.isStopping() {
		return false
	}
	r.channelsMu.RLock()
	defer r.channelsMu.RUnlock()
	if r.channelsClosed {
		return false
	}
	select {
	case r.liveHDHandshakes <- handshake:
		return true
	default:
		return false
	}
}

func (r *AgentRun) PendingLiveHDHandshakes() int {
	if r.isStopping() {
		return 0
	}
	r.channelsMu.RLock()
	defer r.channelsMu.RUnlock()
	if r.channelsClosed {
		return 0
	}
	return len(r.liveHDHandshakes)
}

func (r *AgentRun) TrySendMotion(motion MotionDataPartial) bool {
	if r.isStopping() {
		return false
	}
	r.channelsMu.RLock()
	defer r.channelsMu.RUnlock()
	if r.channelsClosed {
		return false
	}
	select {
	case r.motionEvents <- motion:
		return true
	default:
		return false
	}
}

func (r *AgentRun) TrySendONVIF(action OnvifAction) bool {
	if r.isStopping() {
		return false
	}
	r.channelsMu.RLock()
	defer r.channelsMu.RUnlock()
	if r.channelsClosed {
		return false
	}
	select {
	case r.onvifActions <- action:
		return true
	default:
		return false
	}
}

func (r *AgentRun) Shutdown(ctx context.Context, cause error) AgentRunShutdownReport {
	if ctx == nil {
		ctx = context.Background()
	}
	if cause == nil {
		cause = context.Canceled
	}

	r.shutdownOnce.Do(func() {
		startedAt := time.Now()
		log.WithError(cause).WithFields(log.Fields{
			"component": "agent_run",
			"event":     "shutdown_started",
			"run_id":    r.id,
		}).Info("Agent run shutdown started")

		r.stateMu.Lock()
		r.stopping = true
		activated := r.activated
		r.stateMu.Unlock()

		r.cancel(cause)
		r.supervisor.BeginShutdown(cause)

		if activated && r.stopUpload {
			r.shutdownReport.UploadStopDelivered = sendRunStop(ctx, r.communication.HandleUpload)
		}
		if activated {
			r.shutdownReport.StreamStopDelivered = sendRunStop(ctx, r.communication.HandleStream)
		}

		r.resourcesMu.RLock()
		mainClient := r.mainClient
		subClient := r.subClient
		backchannelClient := r.backchannelClient
		mainQueue := r.mainQueue
		subQueue := r.subQueue
		releaseClients := r.releaseClients
		r.resourcesMu.RUnlock()

		r.closeClient(ctx, "main RTSP client", mainClient)
		if mainQueue != nil {
			_ = mainQueue.Close()
		}
		r.closeClient(ctx, "sub RTSP client", subClient)
		if subQueue != nil {
			_ = subQueue.Close()
		}
		r.closeClient(ctx, "RTSP backchannel client", backchannelClient)
		r.closeChannels()
		if releaseClients != nil {
			releaseClients()
		}

		r.shutdownReport.ShutdownReport = r.supervisor.Wait(ctx)
		if r.shutdownReport.Complete && r.communication != nil {
			r.communication.detachRun(r)
		}

		runningTasks := make([]string, 0, len(r.shutdownReport.Running))
		for _, task := range r.shutdownReport.Running {
			runningTasks = append(runningTasks, task.Name)
		}
		entry := log.WithFields(log.Fields{
			"component":            "agent_run",
			"duration_ms":          time.Since(startedAt).Milliseconds(),
			"resource_error_count": len(r.shutdownReport.ResourceErrors),
			"run_id":               r.id,
			"running_tasks":        runningTasks,
			"task_count":           len(r.shutdownReport.Tasks),
			"upload_stop_sent":     r.shutdownReport.UploadStopDelivered,
			"stream_stop_sent":     r.shutdownReport.StreamStopDelivered,
		})
		if r.shutdownReport.Complete {
			entry.WithField("event", "shutdown_completed").Info("Agent run shutdown completed")
		} else {
			entry.WithField("event", "shutdown_timed_out").Error("Agent run shutdown timed out")
		}
	})

	report := r.shutdownReport
	report.ResourceErrors = append([]AgentRunResourceError(nil), report.ResourceErrors...)
	return report
}

func (r *AgentRun) isStopping() bool {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	return r.stopping
}

func (r *AgentRun) closeClient(ctx context.Context, name string, client AgentRunClient) {
	if client == nil {
		return
	}
	if err := client.Close(ctx); err != nil {
		r.shutdownReport.ResourceErrors = append(r.shutdownReport.ResourceErrors, AgentRunResourceError{
			Resource: name,
			Err:      err,
		})
		log.WithError(err).WithFields(log.Fields{
			"component": "agent_run",
			"event":     "resource_close_failed",
			"resource":  name,
			"run_id":    r.id,
		}).Warn("Failed to close Agent run resource")
		return
	}
	log.WithFields(log.Fields{
		"component": "agent_run",
		"event":     "resource_closed",
		"resource":  name,
		"run_id":    r.id,
	}).Debug("Agent run resource closed")
}

func (r *AgentRun) closeChannels() {
	r.channelsMu.Lock()
	defer r.channelsMu.Unlock()
	if r.channelsClosed {
		return
	}
	r.channelsClosed = true
	close(r.liveHDHandshakes)
	close(r.motionEvents)
	close(r.onvifActions)
}

func sendRunStop(ctx context.Context, channel chan<- string) bool {
	if channel == nil {
		return false
	}
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case channel <- "stop":
		return true
	case <-ctx.Done():
		return false
	case <-timer.C:
		return false
	}
}
