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

// The library handles in-stream reconnect; these guards cover the
// initial-connect path the library cannot see.
const (
	initialBackoff = time.Second
	maxBackoff     = 5 * time.Minute
)

// HandleONVIFEventStream opens an event/stream against the configured
// ONVIF camera and routes Motion events into communication.HandleMotion.
//
// Behind the Capture.ONVIFMotion flag; the goroutine returns
// immediately when not enabled. The flag is read once at start, so
// toggling at runtime requires an agent restart. On transient
// construction failure (camera not yet ready at boot, brief network
// blip, credential reload) the goroutine retries with exponential
// backoff. Exits when ctx is cancelled.
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

// runStreamOnce returns true when the caller should retry construction
// (transient failure), false on clean ctx-driven shutdown.
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

	// recovering = the first successful event after an error streak
	// logs a recovery line so on-call operators see the clear-of-
	// condition for the ERROR they were paged on.
	var recovering bool
	for {
		select {
		case <-ctx.Done():
			return false
		case ev, ok := <-s.Events():
			if !ok {
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

// dispatchEvent routes motion-active events to HandleMotion.
//
// The ctx pre-check + ctx-in-select guards a shutdown race: the agent
// closes HandleMotion shortly after cancelling ctx, and a stale event
// reaching the send would otherwise panic on a closed channel.
func dispatchEvent(ctx context.Context, ev stream.Event, configuration *models.Configuration, communication *models.Communication) {
	if ev.Kind != stream.KindMotion {
		log.Log.Debug("onvif.dispatchEvent(): non-motion event " + ev.Kind.String() + " topic=" + ev.Topic)
		return
	}
	if ev.State != stream.StateActive {
		return
	}
	if configuration.Config.Capture.Recording == "false" {
		return
	}
	if ctx.Err() != nil {
		return
	}
	// The topic that actually started a recording is the one on-call
	// needs; the reject path below already names the ones that didn't.
	log.Log.Debug("onvif.dispatchEvent(): recording trigger " + ev.Kind.String() + " topic=" + ev.Topic)

	dataToPass := models.MotionDataPartial{
		Timestamp:       time.Now().Unix(),
		NumberOfChanges: 0, // ONVIF does not quantify motion area.
	}
	select {
	case <-ctx.Done():
	case communication.HandleMotion <- dataToPass:
	default:
		log.Log.Debug("onvif.dispatchEvent(): HandleMotion full, dropping ONVIF motion event")
	}
}

// logStreamError logs at a level matching severity: recreate is loud
// because it usually means the camera is offline; pull and renew are
// debug because the library recovers from them automatically.
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

func isONVIFMotionEnabled(v string) bool {
	return strings.EqualFold(strings.TrimSpace(v), "true")
}

// resolveDeviceID falls back from operator-supplied name to ONVIF
// endpoint to a constant placeholder so log lines always have
// something to grep.
func resolveDeviceID(configName, xaddr string) string {
	if n := strings.TrimSpace(configName); n != "" {
		return n
	}
	if x := strings.TrimSpace(xaddr); x != "" {
		return x
	}
	return "unknown"
}

// sleepCtx returns false if ctx was cancelled, true if d elapsed.
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
