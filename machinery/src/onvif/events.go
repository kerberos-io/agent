package onvif

import (
	"context"
	"errors"
	"time"

	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/onvif/event/stream"
)

// HandleONVIFEventStream opens an event/stream against the configured
// ONVIF camera and routes Motion events into communication.HandleMotion
// so they trigger the existing recording pipeline.
//
// The goroutine returns when the stream's context is cancelled (the
// caller closes ctx when shutting the agent down) or when the camera
// is not configured for ONVIF.
//
// This is a feature behind Capture.ONVIFMotion. When the flag is empty
// or "false" the goroutine returns immediately, preserving the
// pixel-diff motion detector as the only source.
func HandleONVIFEventStream(ctx context.Context, configuration *models.Configuration, communication *models.Communication) {
	log.Log.Debug("onvif.HandleONVIFEventStream(): started")
	defer log.Log.Debug("onvif.HandleONVIFEventStream(): finished")

	cfg := configuration.Config.Capture
	if cfg.ONVIFMotion != "true" {
		return
	}
	camera := cfg.IPCamera
	if camera.ONVIFXAddr == "" {
		log.Log.Info("onvif.HandleONVIFEventStream(): ONVIFMotion enabled but ONVIFXAddr is empty; nothing to do")
		return
	}

	device, _, err := ConnectToOnvifDevice(&camera)
	if err != nil {
		log.Log.Error("onvif.HandleONVIFEventStream(): connect: " + err.Error())
		return
	}

	s, err := stream.NewStream(ctx, device, stream.Options{
		DeviceID: configuration.Name,
	})
	if err != nil {
		log.Log.Error("onvif.HandleONVIFEventStream(): open stream: " + err.Error())
		return
	}
	defer func() {
		if err := s.Close(); err != nil {
			log.Log.Debug("onvif.HandleONVIFEventStream(): close: " + err.Error())
		}
	}()

	log.Log.Info("onvif.HandleONVIFEventStream(): consuming events for " + configuration.Name)

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-s.Events():
			if !ok {
				return
			}
			dispatchEvent(ev, configuration, communication)
		case e, ok := <-s.Errors():
			if !ok {
				return
			}
			logStreamError(e)
		}
	}
}

// dispatchEvent routes a single decoded ONVIF Event into the agent's
// existing channels. Motion-active events become MotionDataPartial on
// HandleMotion, matching what the pixel-diff detector emits. Other
// event kinds are logged at debug for now; a follow-up will wire
// DigitalInput/Output into the existing inputOutputDeviceMap.
func dispatchEvent(ev stream.Event, configuration *models.Configuration, communication *models.Communication) {
	if ev.Kind != stream.KindMotion {
		log.Log.Debug("onvif.dispatchEvent(): non-motion event " + ev.Kind.String() + " topic=" + ev.Topic)
		return
	}
	// We only fire on the leading edge — StateActive. Motion-stop
	// handling is a follow-up that needs the recorder state machine
	// to accept an explicit stop signal; today the recorder uses a
	// fixed PostRecording timeout.
	if ev.State != stream.StateActive {
		return
	}
	if configuration.Config.Capture.Recording == "false" {
		return
	}
	dataToPass := models.MotionDataPartial{
		Timestamp:       time.Now().Unix(),
		NumberOfChanges: 0, // ONVIF doesn't quantify motion area.
	}
	select {
	case communication.HandleMotion <- dataToPass:
	default:
		log.Log.Debug("onvif.dispatchEvent(): HandleMotion full, dropping ONVIF motion event")
	}
}

// logStreamError logs at a level matching the severity. Recreate
// failures are louder because they usually mean the camera is offline.
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
