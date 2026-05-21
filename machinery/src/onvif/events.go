package onvif

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/onvif/event/stream"
)

// initialBackoff and maxBackoff bound the wait between successive
// attempts to (re)open the event stream after a transient construction
// failure (camera reachable but ONVIF not yet ready, brief network blip
// at agent boot, etc.). The library itself handles in-stream reconnect;
// these guards cover the initial-connect path the library cannot see.
const (
	initialBackoff = time.Second
	maxBackoff     = 5 * time.Minute
)

// HandleONVIFEventStream opens an event/stream against the configured
// ONVIF camera and routes Motion events into communication.HandleMotion
// so they trigger the existing recording pipeline.
//
// The goroutine retries construction with exponential backoff on
// transient failure (camera reachable later, credentials reloaded,
// network restored). It exits cleanly when ctx is cancelled.
//
// This is a feature behind Capture.ONVIFMotion. When the flag is not
// enabled the goroutine returns immediately, preserving the pixel-diff
// motion detector as the only source. Toggling the flag at runtime
// requires an agent restart (Capture.ONVIFMotion is read once at
// goroutine start).
func HandleONVIFEventStream(ctx context.Context, configuration *models.Configuration, communication *models.Communication) {
	log.Log.Debug("onvif.HandleONVIFEventStream(): started")
	defer log.Log.Debug("onvif.HandleONVIFEventStream(): finished")

	if !isONVIFMotionEnabled(configuration.Config.Capture.ONVIFMotion) {
		return
	}
	if configuration.Config.Capture.IPCamera.ONVIFXAddr == "" {
		log.Log.Warning("onvif.HandleONVIFEventStream(): ONVIFMotion enabled but ONVIFXAddr is empty; nothing to do")
		return
	}

	backoff := initialBackoff
	for {
		if ctx.Err() != nil {
			return
		}
		recoverable := runStreamOnce(ctx, configuration, communication)
		if !recoverable {
			return
		}
		if !sleepCtx(ctx, backoff) {
			return
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// runStreamOnce opens one event stream and consumes from it until ctx
// is cancelled or the stream exits. Returns true when the caller
// should retry construction (transient failure), false on clean
// ctx-driven shutdown.
func runStreamOnce(ctx context.Context, configuration *models.Configuration, communication *models.Communication) (retry bool) {
	camera := configuration.Config.Capture.IPCamera

	device, _, err := ConnectToOnvifDevice(&camera)
	if err != nil {
		log.Log.Error("onvif.HandleONVIFEventStream(): connect: " + err.Error())
		return true
	}

	deviceID := resolveDeviceID(configuration.Name, camera.ONVIFXAddr)
	s, err := stream.NewStream(ctx, device, stream.Options{DeviceID: deviceID})
	if err != nil {
		log.Log.Error("onvif.HandleONVIFEventStream(): open stream: " + err.Error())
		return true
	}
	defer func() {
		if err := s.Close(); err != nil {
			log.Log.Debug("onvif.HandleONVIFEventStream(): close: " + err.Error())
		}
	}()

	log.Log.Info("onvif.HandleONVIFEventStream(): consuming events for " + deviceID)

	// recovering tracks whether we're in a degraded period so the
	// first successful event after an error streak can log a recovery
	// line for on-call operators.
	var recovering bool
	for {
		select {
		case <-ctx.Done():
			return false
		case ev, ok := <-s.Events():
			if !ok {
				// Lib's run goroutine exited — happens only on ctx
				// cancel today (library handles its own reconnect),
				// so treat as clean shutdown.
				return false
			}
			if recovering {
				log.Log.Info("onvif.HandleONVIFEventStream(): event stream recovered for " + deviceID)
				recovering = false
			}
			dispatchEvent(ctx, ev, configuration, communication)
		case e, ok := <-s.Errors():
			if !ok {
				return false
			}
			recovering = true
			logStreamError(e)
		}
	}
}

// dispatchEvent routes a single decoded ONVIF Event into the agent's
// existing channels. Motion-active events become MotionDataPartial on
// HandleMotion, matching what the pixel-diff detector emits.
//
// The ctx pre-check is the shutdown-race guard: between cancel() and
// close(communication.HandleMotion) the agent leaves a ~3s window in
// which a stale event could otherwise attempt to send on a closed
// channel and panic. If ctx is done we drop the event silently — the
// recording pipeline is already winding down.
func dispatchEvent(ctx context.Context, ev stream.Event, configuration *models.Configuration, communication *models.Communication) {
	if ev.Kind != stream.KindMotion {
		log.Log.Debug("onvif.dispatchEvent(): non-motion event " + ev.Kind.String() + " topic=" + ev.Topic)
		return
	}
	// Leading-edge only. Motion-stop wiring into the recorder state
	// machine is tracked as a follow-up; today the recorder uses a
	// fixed PostRecording timeout.
	if ev.State != stream.StateActive {
		return
	}
	if configuration.Config.Capture.Recording == "false" {
		return
	}
	if ctx.Err() != nil {
		return
	}
	// Timestamp in seconds matches what computervision/main.go emits;
	// downstream consumers (capture/main.go) tolerate either second-
	// or millisecond-precision.
	dataToPass := models.MotionDataPartial{
		Timestamp:       time.Now().Unix(),
		NumberOfChanges: 0, // ONVIF does not quantify motion area.
	}
	select {
	case <-ctx.Done():
		// Closes the residual race: ctx cancelled between the
		// pre-check above and reaching this select.
	case communication.HandleMotion <- dataToPass:
	default:
		log.Log.Debug("onvif.dispatchEvent(): HandleMotion full, dropping ONVIF motion event")
	}
}

// logStreamError logs at a level matching the severity. Recreate
// failures are loud because they usually mean the camera is offline;
// pull/renew failures are debug because the library recovers from them
// automatically.
func logStreamError(e error) {
	var recreate stream.ErrRecreateFailed
	var pull stream.ErrPullFailed
	var renew stream.ErrRenewFailed
	switch {
	case errors.As(e, &recreate):
		log.Log.Error("onvif.HandleONVIFEventStream(): subscription recreate failed (camera may be offline): " + recreate.Err.Error())
	case errors.As(e, &renew):
		log.Log.Debug("onvif.HandleONVIFEventStream(): renew failed (will recover via pull/recreate): " + renew.Err.Error())
	case errors.As(e, &pull):
		log.Log.Debug("onvif.HandleONVIFEventStream(): pull failed (will retry): " + pull.Err.Error())
	default:
		log.Log.Info("onvif.HandleONVIFEventStream(): stream error: " + e.Error())
	}
}

// isONVIFMotionEnabled returns true when the Capture.ONVIFMotion flag
// is set to "true" with case and whitespace tolerance. The rest of the
// Capture struct uses string flags so we keep the same shape; the
// difference is that ONVIFMotion defaults to disabled (opt-in), unlike
// Recording / Motion / Snapshots which default to enabled.
func isONVIFMotionEnabled(v string) bool {
	return strings.EqualFold(strings.TrimSpace(v), "true")
}

// resolveDeviceID returns the most useful identifier for the camera
// in stream events, logs and metrics. Falls back from configuration
// name (the operator-supplied label) to the ONVIF endpoint to a
// constant placeholder so log lines always have something to grep.
func resolveDeviceID(configName, xaddr string) string {
	if n := strings.TrimSpace(configName); n != "" {
		return n
	}
	if x := strings.TrimSpace(xaddr); x != "" {
		return x
	}
	return "unknown"
}

// sleepCtx blocks for d or until ctx is cancelled. Returns false if
// ctx was cancelled, true if the full duration elapsed.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
