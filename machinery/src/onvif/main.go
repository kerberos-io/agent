package onvif

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/onvif"
	"github.com/kerberos-io/onvif/device"
	"github.com/kerberos-io/onvif/deviceio"
	"github.com/kerberos-io/onvif/event"
	"github.com/kerberos-io/onvif/media"
	"github.com/kerberos-io/onvif/ptz"
	xsd "github.com/kerberos-io/onvif/xsd"
	xsdonvif "github.com/kerberos-io/onvif/xsd/onvif"
	log "github.com/sirupsen/logrus"
)

// Discover performs an advanced Fing/WiFiman-style scan of the local network
// (ONVIF WS-Discovery + active port scan + MAC/vendor lookup) and prints a
// human readable summary of everything it finds. It is used by the
// `-action discover` CLI command. Optional subnets (CIDR, e.g.
// "192.168.1.0/24") override the auto-detected local subnets.
func Discover(timeout time.Duration, subnets ...string) {
	log.Info("onvif.Discover(): starting advanced network discovery")
	log.Info("onvif.Discover(): this may take up to " + timeout.String() + " for the ONVIF probe plus the port scan")

	devices := DiscoverDevices(timeout, subnets...)
	if len(devices) == 0 {
		log.Info("onvif.Discover(): no devices discovered on the local network")
		return
	}

	cameraCount := 0
	for _, device := range devices {
		if device.IsCamera {
			cameraCount++
		}
	}
	log.Info("onvif.Discover(): found " + strconv.Itoa(len(devices)) + " device(s), " + strconv.Itoa(cameraCount) + " likely camera(s)")

	for _, device := range devices {
		label := "device"
		if device.IsCamera {
			label = "camera"
		} else if device.IsAudio {
			label = "speaker"
		}
		summary := "onvif.Discover(): [" + label + "] " + device.IP
		if device.Hostname != "" {
			summary += " (" + device.Hostname + ")"
		}
		if device.MAC != "" {
			summary += " mac=" + device.MAC
		}
		if device.Vendor != "" {
			summary += " vendor=" + device.Vendor
		}
		if device.Type != "" {
			summary += " type=" + device.Type
		}
		if device.Manufacturer != "" {
			summary += " manufacturer=" + device.Manufacturer
		}
		if device.Model != "" {
			summary += " model=" + device.Model
		}
		if device.Server != "" {
			summary += " server=\"" + device.Server + "\""
		}
		if device.ONVIF {
			summary += " onvif=" + device.ONVIFXAddr
		}
		if len(device.Services) > 0 {
			summary += " services=[" + strings.Join(device.Services, ", ") + "]"
		}
		if device.RTSPURL != "" {
			summary += " rtsp=" + device.RTSPURL
		}
		log.Info(summary)

		// Detail the guessed RTSP stream URLs from the brand -> RTSP mapping.
		for _, stream := range device.RTSPStreams {
			status := "guess"
			if stream.Verified {
				status = "confirmed"
			}
			line := "onvif.Discover():   -> " + stream.Stream + " stream [" + status + "]"
			if stream.RequiresAuth {
				line += " (auth required)"
			}
			line += ": " + stream.URL
			log.Info(line)
		}
	}
}

func HandleONVIFActions(configuration *models.Configuration, communication *models.Communication, actions <-chan models.OnvifAction) {
	log.Debug("onvif.HandleONVIFActions(): started")

	for onvifAction := range actions {

		// First we'll get the desired PTZ action from the payload
		// We need to know if we need to move left, right, up, down, zoom in, zoom out, center.
		var ptzAction models.OnvifActionPTZ
		b, _ := json.Marshal(onvifAction.Payload)
		json.Unmarshal(b, &ptzAction)

		// Connect to Onvif device
		cameraConfiguration := configuration.Config.Capture.IPCamera
		device, _, err := ConnectToOnvifDevice(&cameraConfiguration)
		if err == nil {

			// Get token from the first profile
			token, err := GetTokenFromProfile(device, 0)
			if err == nil {

				// Get the configurations from the device
				configurations, err := GetPTZConfigurationsFromDevice(device)

				if err == nil {

					if onvifAction.Action == "absolute-move" {

						// Move the camera to the saved absolute position.
						x := ptzAction.X
						y := ptzAction.Y
						z := ptzAction.Z

						// Check which PTZ Space we need to use
						functions, _, _ := GetPTZFunctionsFromDevice(configurations)

						// Log functions
						log.Debug("onvif.HandleONVIFActions(): functions: " + strings.Join(functions, ", "))

						err = MoveToPosition(device, configurations, token, x, y, z)
						if err != nil {
							log.Error("onvif.HandleONVIFActions() - MoveToPosition: " + err.Error())
						} else {
							log.Info("onvif.HandleONVIFActions() - MoveToPosition: successfully moved camera.")
						}

					} else if onvifAction.Action == "preset" {

						// Execute the preset
						preset := ptzAction.Preset
						err := GoToPresetFromDevice(device, preset)
						if err != nil {
							log.Debug("onvif.HandleONVIFActions() - GotoPreset: " + err.Error())
						} else {
							log.Info("onvif.HandleONVIFActions() - GotoPreset: successfully moved camera")
						}

					} else if onvifAction.Action == "ptz" {

						if err == nil {

							if ptzAction.Center == 1 {

								// We will move the camera to zero position.
								err := AbsolutePanTiltMove(device, configurations, token, 0, 0, 0)
								if err != nil {
									log.Debug("onvif.HandleONVIFActions() - AbsolutePanTitleMove: " + err.Error())
								} else {
									log.Info("onvif.HandleONVIFActions() - AbsolutePanTitleMove: successfully centered camera")
								}

							} else {

								// Distance should be a parameter as well
								distance := 0.7

								// We will calculate if we need to move pan or tilt (and the direction).
								x := float64(0)
								y := float64(0)

								if ptzAction.Left == 1 {
									x = -1 * distance
								}
								if ptzAction.Right == 1 {
									x = 1 * distance
								}
								if ptzAction.Up == 1 {
									y = 1 * distance
								}
								if ptzAction.Down == 1 {
									y = -1 * distance
								}

								err := ContinuousPanTilt(device, configurations, token, x, y)
								if err != nil {
									log.Debug("onvif.HandleONVIFActions() - ContinuousPanTilt: " + err.Error())
								} else {
									log.Info("onvif.HandleONVIFActions() - ContinuousPanTilt: successfully pan tilted camera")
								}
							}
						}
					} else if onvifAction.Action == "zoom" {

						if err == nil {
							zoom := ptzAction.Zoom
							err := ContinuousZoom(device, configurations, token, zoom)
							if err != nil {
								log.Debug("onvif.HandleONVIFActions() - ContinuousZoom: " + err.Error())
							} else {
								log.Info("onvif.HandleONVIFActions() - ContinuousZoom: successfully zoomed camera")
							}
						}
					}
				}
			}
		}
	}
	log.Debug("onvif.HandleONVIFActions(): finished")
}

func ConnectToOnvifDevice(cameraConfiguration *models.IPCamera) (*onvif.Device, device.GetCapabilitiesResponse, error) {
	log.Debug("onvif.ConnectToOnvifDevice(): started")
	dev, err := onvif.NewDevice(onvif.DeviceParams{
		Xaddr:    cameraConfiguration.ONVIFXAddr,
		Username: cameraConfiguration.ONVIFUsername,
		Password: cameraConfiguration.ONVIFPassword,
		AuthMode: "both",
	})

	var capabilities device.GetCapabilitiesResponse
	if err != nil {
		// Try again with other authentication mode
		dev, err = onvif.NewDevice(onvif.DeviceParams{
			Xaddr:    cameraConfiguration.ONVIFXAddr,
			Username: cameraConfiguration.ONVIFUsername,
			Password: cameraConfiguration.ONVIFPassword,
			AuthMode: "digest",
		})
		if err != nil {
			log.Debug("onvif.ConnectToOnvifDevice(): " + err.Error())
		}
	}

	if err == nil {
		getCapabilities := device.GetCapabilities{Category: []xsdonvif.CapabilityCategory{"All"}}
		resp, err := dev.CallMethod(getCapabilities)
		if err != nil {
			log.Error("onvif.ConnectToOnvifDevice(): " + err.Error())
		}

		var b []byte
		if resp != nil {
			b, err = io.ReadAll(resp.Body)
			resp.Body.Close() // Ensure the response body is closed
			if err != nil {
				log.Error("onvif.ConnectToOnvifDevice(): " + err.Error())
			}
		}
		stringBody := string(b)
		decodedXML, et, err := getXMLNode(stringBody, "GetCapabilitiesResponse")
		if err != nil {
			log.Error("onvif.ConnectToOnvifDevice(): " + err.Error())
		} else {
			if err := decodedXML.DecodeElement(&capabilities, et); err != nil {
				log.Error("onvif.ConnectToOnvifDevice(): " + err.Error())
			} else {
				log.Debug("onvif.ConnectToOnvifDevice(): capabilities.")
			}
		}

		log.Info("onvif.ConnectToOnvifDevice(): successfully connected to device")
	}
	log.Debug("onvif.ConnectToOnvifDevice(): finished")
	return dev, capabilities, err
}

func GetTokenFromProfile(device *onvif.Device, profileId int) (xsdonvif.ReferenceToken, error) {
	// We aim to receive a profile token from the server
	var profileToken xsdonvif.ReferenceToken

	// Get Profiles
	resp, err := device.CallMethod(media.GetProfiles{})
	if err == nil {
		b, err := io.ReadAll(resp.Body)
		if err == nil {
			stringBody := string(b)
			resp.Body.Close() // Ensure the response body is closed
			decodedXML, et, err := getXMLNode(stringBody, "GetProfilesResponse")
			if err != nil {
				log.Debug("onvif.GetTokenFromProfile(): " + err.Error())
				return profileToken, err
			} else {
				// Decode the profiles from the server
				var mProfilesResp media.GetProfilesResponse
				if err := decodedXML.DecodeElement(&mProfilesResp, et); err != nil {
					log.Debug("onvif.GetTokenFromProfile(): " + err.Error())
				}

				// We'll try to get the token from a preferred profile
				for i, profile := range mProfilesResp.Profiles {
					if profileId == i {
						profileToken = profile.Token
					}
				}
			}
		}
	}
	return profileToken, err
}

func GetPTZConfigurationsFromDevice(device *onvif.Device) (ptz.GetConfigurationsResponse, error) {
	// We'll try to receive the PTZ configurations from the server
	var configurations ptz.GetConfigurationsResponse

	// Get the PTZ configurations from the device
	resp, err := device.CallMethod(ptz.GetConfigurations{})
	var b []byte
	if resp != nil {
		b, err = io.ReadAll(resp.Body)
		resp.Body.Close() // Ensure the response body is closed
	}

	if err == nil {
		stringBody := string(b)
		decodedXML, et, err := getXMLNode(stringBody, "GetConfigurationsResponse")
		if err != nil {
			log.Debug("onvif.GetPTZConfigurationsFromDevice(): " + err.Error())
			return configurations, err
		} else {
			if err := decodedXML.DecodeElement(&configurations, et); err != nil {
				log.Debug("onvif.GetPTZConfigurationsFromDevice(): " + err.Error())
				return configurations, err
			}
		}
	}
	return configurations, err
}

func GetPositionFromDevice(configuration models.Configuration) (xsdonvif.PTZVector, error) {
	var position xsdonvif.PTZVector
	// Connect to Onvif device
	cameraConfiguration := configuration.Config.Capture.IPCamera
	device, _, err := ConnectToOnvifDevice(&cameraConfiguration)
	if err == nil {

		// Get token from the first profile
		token, err := GetTokenFromProfile(device, 0)
		if err == nil {
			// Get the PTZ configurations from the device
			position, err := GetPosition(device, token)
			if err == nil {
				if position.PanTilt != nil && position.Zoom != nil {
					// float to string
					x := strconv.FormatFloat(position.PanTilt.X, 'f', 6, 64)
					y := strconv.FormatFloat(position.PanTilt.Y, 'f', 6, 64)
					z := strconv.FormatFloat(position.Zoom.X, 'f', 6, 64)
					log.Info("onvif.GetPositionFromDevice(): successfully got position (" + x + ", " + y + ", " + z + ")")
					return position, err
				} else {
					log.Debug("onvif.GetPositionFromDevice(): position is nil")
					return position, errors.New("position is nil")
				}
			} else {
				log.Debug("onvif.GetPositionFromDevice(): " + err.Error())
				return position, err
			}
		} else {
			log.Debug("onvif.GetPositionFromDevice(): " + err.Error())
			return position, err
		}
	} else {
		log.Debug("onvif.GetPositionFromDevice(): " + err.Error())
		return position, err
	}
}

func GetPosition(device *onvif.Device, token xsdonvif.ReferenceToken) (xsdonvif.PTZVector, error) {
	// We'll try to receive the PTZ configurations from the server
	var status ptz.GetStatusResponse
	var position xsdonvif.PTZVector

	// Get the PTZ configurations from the device
	resp, err := device.CallMethod(ptz.GetStatus{
		ProfileToken: token,
	})

	var b []byte
	if resp != nil {
		b, err = io.ReadAll(resp.Body)
		resp.Body.Close() // Ensure the response body is closed
	}

	if err == nil {
		stringBody := string(b)
		decodedXML, et, err := getXMLNode(stringBody, "GetStatusResponse")
		if err != nil {
			log.Error("GetPositionFromDevice: " + err.Error())
			return position, err
		} else {
			if err := decodedXML.DecodeElement(&status, et); err != nil {
				log.Error("GetPositionFromDevice: " + err.Error())
				return position, err
			}
		}
	}
	position = status.PTZStatus.Position
	return position, err
}

func handleONVIFResponse(operation string, response *http.Response, requestErr error) error {
	fields := log.Fields{
		"component": "onvif",
		"operation": operation,
	}
	var responseErr error
	if response != nil {
		fields["status_code"] = response.StatusCode
		fields["response_bytes"], responseErr = io.Copy(io.Discard, response.Body)
		response.Body.Close()
	}

	err := errors.Join(requestErr, responseErr)
	if response == nil && err == nil {
		err = errors.New("ONVIF operation returned no response")
	}
	if err == nil && (response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices) {
		err = errors.New("ONVIF operation was rejected")
	}
	if err != nil {
		fields["event"] = "operation_failed"
		log.WithError(err).WithFields(fields).Error("ONVIF operation failed")
		return err
	}

	fields["event"] = "operation_completed"
	log.WithFields(fields).Debug("ONVIF operation completed")
	return nil
}

func AbsolutePanTiltMove(device *onvif.Device, configuration ptz.GetConfigurationsResponse, token xsdonvif.ReferenceToken, pan float64, tilt float64, zoom float64) error {
	if len(configuration.PTZConfiguration) == 0 {
		return errors.New("camera returned no PTZ configuration")
	}
	ptzConfiguration := configuration.PTZConfiguration[0]

	absolutePantiltVector := xsdonvif.Vector2D{
		X:     pan,
		Y:     tilt,
		Space: ptzConfiguration.DefaultAbsolutePantTiltPositionSpace,
	}

	absoluteZoomVector := xsdonvif.Vector1D{
		X:     zoom,
		Space: ptzConfiguration.DefaultAbsoluteZoomPositionSpace,
	}

	resp, err := device.CallMethod(ptz.AbsoluteMove{
		ProfileToken: token,
		Position: xsdonvif.PTZVector{
			PanTilt: &absolutePantiltVector,
			Zoom:    &absoluteZoomVector,
		},
	})
	return handleONVIFResponse("absolute_move", resp, err)
}

type ptzMoveClient interface {
	GetPosition() (xsdonvif.PTZVector, error)
	AbsoluteMove(ptzTarget) error
	ContinuousMove(ptzVelocity) error
	Stop() error
}

type ptzTarget struct {
	Pan  float64
	Tilt float64
	Zoom float64
}

type ptzVelocity struct {
	Pan  float64
	Tilt float64
	Zoom float64
}

type ptzMoveSupport struct {
	Absolute   bool
	Continuous bool
}

type ptzMoveOptions struct {
	PanTolerance      float64
	TiltTolerance     float64
	ZoomTolerance     float64
	NativeTimeout     time.Duration
	ContinuousTimeout time.Duration
	NoProgressTimeout time.Duration
	PollInterval      time.Duration
	ProgressEpsilon   float64
	VelocityGain      float64
	MinVelocity       float64
	MaxVelocity       float64
	VelocityChange    float64
}

var defaultPTZMoveOptions = ptzMoveOptions{
	PanTolerance:      0.01,
	TiltTolerance:     0.005,
	ZoomTolerance:     0.005,
	NativeTimeout:     8 * time.Second,
	ContinuousTimeout: 15 * time.Second,
	NoProgressTimeout: 3 * time.Second,
	PollInterval:      150 * time.Millisecond,
	ProgressEpsilon:   0.001,
	VelocityGain:      1.5,
	MinVelocity:       0.08,
	MaxVelocity:       0.7,
	VelocityChange:    0.05,
}

type devicePTZMoveClient struct {
	device        *onvif.Device
	configuration xsdonvif.PTZConfiguration
	token         xsdonvif.ReferenceToken
}

func (client *devicePTZMoveClient) GetPosition() (xsdonvif.PTZVector, error) {
	return GetPosition(client.device, client.token)
}

func (client *devicePTZMoveClient) AbsoluteMove(target ptzTarget) error {
	configuration := ptz.GetConfigurationsResponse{
		PTZConfiguration: []xsdonvif.PTZConfiguration{client.configuration},
	}
	return AbsolutePanTiltMove(client.device, configuration, client.token, target.Pan, target.Tilt, target.Zoom)
}

func (client *devicePTZMoveClient) ContinuousMove(velocity ptzVelocity) error {
	panTilt := xsdonvif.Vector2D{
		X:     velocity.Pan,
		Y:     velocity.Tilt,
		Space: client.configuration.DefaultContinuousPanTiltVelocitySpace,
	}
	zoom := xsdonvif.Vector1D{
		X:     velocity.Zoom,
		Space: client.configuration.DefaultContinuousZoomVelocitySpace,
	}
	resp, err := client.device.CallMethod(ptz.ContinuousMove{
		ProfileToken: &client.token,
		Velocity: ptz.Speed{
			PanTilt: &panTilt,
			Zoom:    &zoom,
		},
	})
	return handleONVIFResponse("continuous_move_to_position", resp, err)
}

func (client *devicePTZMoveClient) Stop() error {
	resp, err := client.device.CallMethod(ptz.Stop{
		ProfileToken: client.token,
		PanTilt:      true,
		Zoom:         true,
	})
	return handleONVIFResponse("move_to_position_stop", resp, err)
}

func MoveToPosition(device *onvif.Device, configuration ptz.GetConfigurationsResponse, token xsdonvif.ReferenceToken, pan float64, tilt float64, zoom float64) error {
	if len(configuration.PTZConfiguration) == 0 {
		return errors.New("camera returned no PTZ configuration")
	}
	ptzConfiguration := configuration.PTZConfiguration[0]
	client := &devicePTZMoveClient{
		device:        device,
		configuration: ptzConfiguration,
		token:         token,
	}
	support := ptzMoveSupport{
		Absolute: ptzConfiguration.DefaultAbsolutePantTiltPositionSpace != nil &&
			ptzConfiguration.DefaultAbsoluteZoomPositionSpace != nil,
		Continuous: ptzConfiguration.DefaultContinuousPanTiltVelocitySpace != nil &&
			ptzConfiguration.DefaultContinuousZoomVelocitySpace != nil,
	}
	return moveToPosition(client, support, ptzTarget{Pan: pan, Tilt: tilt, Zoom: zoom}, defaultPTZMoveOptions)
}

// AbsolutePanTiltMoveFake preserves the continuous-move compatibility path for callers that explicitly request it.
func AbsolutePanTiltMoveFake(device *onvif.Device, configuration ptz.GetConfigurationsResponse, token xsdonvif.ReferenceToken, pan float64, tilt float64, zoom float64) error {
	if len(configuration.PTZConfiguration) == 0 {
		return errors.New("camera returned no PTZ configuration")
	}
	ptzConfiguration := configuration.PTZConfiguration[0]
	client := &devicePTZMoveClient{
		device:        device,
		configuration: ptzConfiguration,
		token:         token,
	}
	support := ptzMoveSupport{
		Continuous: ptzConfiguration.DefaultContinuousPanTiltVelocitySpace != nil &&
			ptzConfiguration.DefaultContinuousZoomVelocitySpace != nil,
	}
	return moveToPosition(client, support, ptzTarget{Pan: pan, Tilt: tilt, Zoom: zoom}, defaultPTZMoveOptions)
}

func moveToPosition(client ptzMoveClient, support ptzMoveSupport, target ptzTarget, options ptzMoveOptions) error {
	if err := validatePTZTarget(target); err != nil {
		return err
	}

	position, err := client.GetPosition()
	if err != nil {
		return fmt.Errorf("read initial PTZ position: %w", err)
	}
	_, _, reached, err := ptzTargetError(position, target, options)
	if err != nil {
		return err
	}
	if reached {
		return nil
	}

	var nativeErr error
	if support.Absolute {
		if err := client.AbsoluteMove(target); err != nil {
			nativeErr = fmt.Errorf("native absolute move: %w", err)
		} else if err := waitForPTZTarget(client, target, options.NativeTimeout, options); err != nil {
			nativeErr = fmt.Errorf("verify native absolute move: %w", err)
		} else {
			return nil
		}

		if err := client.Stop(); err != nil {
			nativeErr = errors.Join(nativeErr, fmt.Errorf("stop native absolute move: %w", err))
		}
		log.WithError(nativeErr).Warn("Native ONVIF absolute move failed; using continuous-move fallback")
	}

	if !support.Continuous {
		if nativeErr != nil {
			return errors.Join(nativeErr, errors.New("camera does not advertise complete continuous PTZ support"))
		}
		return errors.New("camera does not advertise a supported absolute or continuous PTZ movement method")
	}

	if err := moveContinuouslyToPTZTarget(client, target, options); err != nil {
		if nativeErr != nil {
			return errors.Join(nativeErr, fmt.Errorf("continuous-move fallback: %w", err))
		}
		return fmt.Errorf("continuous move: %w", err)
	}
	return nil
}

func waitForPTZTarget(client ptzMoveClient, target ptzTarget, timeout time.Duration, options ptzMoveOptions) error {
	deadline := time.Now().Add(timeout)
	lastProgress := time.Now()
	bestDistance := math.Inf(1)

	for {
		position, err := client.GetPosition()
		if err != nil {
			return fmt.Errorf("read PTZ position: %w", err)
		}
		_, distance, reached, err := ptzTargetError(position, target, options)
		if err != nil {
			return err
		}
		if reached {
			return nil
		}

		now := time.Now()
		if bestDistance-distance >= options.ProgressEpsilon {
			bestDistance = distance
			lastProgress = now
		}
		if !now.Before(deadline) {
			return fmt.Errorf("target not reached within %s", timeout)
		}
		if now.Sub(lastProgress) >= options.NoProgressTimeout {
			return fmt.Errorf("PTZ position made no progress for %s", options.NoProgressTimeout)
		}
		time.Sleep(options.PollInterval)
	}
}

func moveContinuouslyToPTZTarget(client ptzMoveClient, target ptzTarget, options ptzMoveOptions) (err error) {
	defer func() {
		if stopErr := client.Stop(); stopErr != nil {
			err = errors.Join(err, fmt.Errorf("stop continuous move: %w", stopErr))
		}
	}()

	deadline := time.Now().Add(options.ContinuousTimeout)
	lastProgress := time.Now()
	bestDistance := math.Inf(1)
	var lastVelocity ptzVelocity
	haveVelocity := false

	for {
		position, positionErr := client.GetPosition()
		if positionErr != nil {
			return fmt.Errorf("read PTZ position: %w", positionErr)
		}
		delta, distance, reached, targetErr := ptzTargetError(position, target, options)
		if targetErr != nil {
			return targetErr
		}
		if reached {
			return nil
		}

		now := time.Now()
		if bestDistance-distance >= options.ProgressEpsilon {
			bestDistance = distance
			lastProgress = now
		}
		if !now.Before(deadline) {
			return fmt.Errorf("target not reached within %s", options.ContinuousTimeout)
		}
		if now.Sub(lastProgress) >= options.NoProgressTimeout {
			return fmt.Errorf("PTZ position made no progress for %s", options.NoProgressTimeout)
		}

		velocity := ptzVelocity{
			Pan:  proportionalPTZVelocity(delta.Pan, options.PanTolerance, options),
			Tilt: proportionalPTZVelocity(delta.Tilt, options.TiltTolerance, options),
			Zoom: proportionalPTZVelocity(delta.Zoom, options.ZoomTolerance, options),
		}
		if !haveVelocity || ptzVelocityChanged(lastVelocity, velocity, options.VelocityChange) {
			if moveErr := client.ContinuousMove(velocity); moveErr != nil {
				return fmt.Errorf("set continuous PTZ velocity: %w", moveErr)
			}
			lastVelocity = velocity
			haveVelocity = true
		}
		time.Sleep(options.PollInterval)
	}
}

func ptzTargetError(position xsdonvif.PTZVector, target ptzTarget, options ptzMoveOptions) (ptzVelocity, float64, bool, error) {
	if position.PanTilt == nil || position.Zoom == nil {
		return ptzVelocity{}, 0, false, errors.New("camera returned an incomplete PTZ position")
	}
	delta := ptzVelocity{
		Pan:  target.Pan - position.PanTilt.X,
		Tilt: target.Tilt - position.PanTilt.Y,
		Zoom: target.Zoom - position.Zoom.X,
	}
	reached := math.Abs(delta.Pan) <= options.PanTolerance &&
		math.Abs(delta.Tilt) <= options.TiltTolerance &&
		math.Abs(delta.Zoom) <= options.ZoomTolerance
	distance := math.Abs(delta.Pan) + math.Abs(delta.Tilt) + math.Abs(delta.Zoom)
	return delta, distance, reached, nil
}

func proportionalPTZVelocity(delta float64, tolerance float64, options ptzMoveOptions) float64 {
	if math.Abs(delta) <= tolerance {
		return 0
	}
	speed := math.Abs(delta) * options.VelocityGain
	speed = math.Max(options.MinVelocity, math.Min(speed, options.MaxVelocity))
	return math.Copysign(speed, delta)
}

func ptzVelocityChanged(previous ptzVelocity, next ptzVelocity, threshold float64) bool {
	return math.Abs(previous.Pan-next.Pan) >= threshold ||
		math.Abs(previous.Tilt-next.Tilt) >= threshold ||
		math.Abs(previous.Zoom-next.Zoom) >= threshold
}

func validatePTZTarget(target ptzTarget) error {
	values := []struct {
		name  string
		value float64
	}{
		{name: "pan", value: target.Pan},
		{name: "tilt", value: target.Tilt},
		{name: "zoom", value: target.Zoom},
	}
	for _, value := range values {
		if math.IsNaN(value.value) || math.IsInf(value.value, 0) {
			return fmt.Errorf("invalid PTZ %s target", value.name)
		}
	}
	return nil
}

func ContinuousPanTilt(device *onvif.Device, configuration ptz.GetConfigurationsResponse, token xsdonvif.ReferenceToken, pan float64, tilt float64) error {

	panTiltVector := xsdonvif.Vector2D{
		X:     pan,
		Y:     tilt,
		Space: configuration.PTZConfiguration[0].DefaultContinuousPanTiltVelocitySpace,
	}

	resp, err := device.CallMethod(ptz.ContinuousMove{
		ProfileToken: &token,
		Velocity: xsdonvif.PTZSpeedPanTilt{
			PanTilt: panTiltVector,
		},
	})
	moveErr := handleONVIFResponse("continuous_pan_tilt", resp, err)

	time.Sleep(200 * time.Millisecond)

	resp, err = device.CallMethod(ptz.Stop{
		ProfileToken: token,
		PanTilt:      true,
	})
	stopErr := handleONVIFResponse("continuous_pan_tilt_stop", resp, err)
	return errors.Join(moveErr, stopErr)
}

func ContinuousZoom(device *onvif.Device, configuration ptz.GetConfigurationsResponse, token xsdonvif.ReferenceToken, zoom float64) error {

	zoomVector := xsdonvif.Vector1D{
		X:     zoom,
		Space: configuration.PTZConfiguration[0].DefaultContinuousZoomVelocitySpace,
	}

	velocity := xsdonvif.PTZSpeedZoom{
		Zoom: zoomVector,
	}

	resp, err := device.CallMethod(ptz.ContinuousMove{
		ProfileToken: &token,
		Velocity:     &velocity,
	})
	moveErr := handleONVIFResponse("continuous_zoom", resp, err)
	time.Sleep(500 * time.Millisecond)

	resp, err = device.CallMethod(ptz.Stop{
		ProfileToken: token,
		Zoom:         true,
	})
	stopErr := handleONVIFResponse("continuous_zoom_stop", resp, err)
	return errors.Join(moveErr, stopErr)
}

func GetCapabilitiesFromDevice(dev *onvif.Device) []string {
	var capabilities []string
	services := dev.GetServices()
	for key, _ := range services {
		log.WithFields(log.Fields{
			"capability": key,
			"component":  "onvif",
			"event":      "capability_detected",
		}).Debug("ONVIF capability detected")
		if key != "" {
			keyParts := strings.Split(key, "/")
			if len(keyParts) > 0 {
				capability := keyParts[len(keyParts)-1]
				capabilities = append(capabilities, capability)
			}
		}
	}
	return capabilities
}

func GetPresetsFromDevice(device *onvif.Device) ([]models.OnvifActionPreset, error) {
	var presets []models.OnvifActionPreset
	var presetsResponse ptz.GetPresetsResponse

	// Get token from the first profile
	token, err := GetTokenFromProfile(device, 0)
	if err == nil {
		resp, err := device.CallMethod(ptz.GetPresets{
			ProfileToken: token,
		})
		var b []byte
		if resp != nil {
			b, err = io.ReadAll(resp.Body)
			resp.Body.Close() // Ensure the response body is closed
		}
		if err == nil {
			stringBody := string(b)
			decodedXML, et, err := getXMLNode(stringBody, "GetPresetsResponse")
			if err != nil {
				log.Error("onvif.main.GetPresetsFromDevice(): " + err.Error())
				return presets, err
			} else {
				if err := decodedXML.DecodeElement(&presetsResponse, et); err != nil {
					log.Error("onvif.main.GetPresetsFromDevice(): " + err.Error())
					return presets, err
				}

				presetsList := ""
				for _, preset := range presetsResponse.Preset {
					p := models.OnvifActionPreset{
						Name:  string(preset.Name),
						Token: string(preset.Token),
					}
					presetsList += string(preset.Name) + " (" + string(preset.Token) + "), "
					presets = append(presets, p)
				}
				log.Debug("onvif.main.GetPresetsFromDevice(): " + presetsList)

				return presets, err
			}
		} else {
			log.Error("onvif.main.GetPresetsFromDevice(): " + err.Error())
		}
	} else {
		log.Error("onvif.main.GetPresetsFromDevice(): " + err.Error())
	}

	return presets, err
}

func GoToPresetFromDevice(device *onvif.Device, presetName string) error {
	var goToPresetResponse ptz.GotoPresetResponse

	// Get token from the first profile
	token, err := GetTokenFromProfile(device, 0)
	if err == nil {
		preset := xsdonvif.ReferenceToken(presetName)
		resp, err := device.CallMethod(ptz.GotoPreset{
			ProfileToken: &token,
			PresetToken:  &preset,
		})
		var b []byte
		if resp != nil {
			b, err = io.ReadAll(resp.Body)
			resp.Body.Close() // Ensure the response body is closed
		}
		if err == nil {
			stringBody := string(b)
			decodedXML, et, err := getXMLNode(stringBody, "GotoPresetResponses")
			if err != nil {
				log.Error("onvif.main.GoToPresetFromDevice(): " + err.Error())
				return err
			} else {
				if err := decodedXML.DecodeElement(&goToPresetResponse, et); err != nil {
					log.Error("onvif.main.GoToPresetFromDevice(): " + err.Error())
					return err
				}
				return err
			}
		} else {
			log.Error("onvif.main.GoToPresetFromDevice(): " + err.Error())
		}
	} else {
		log.Error("onvif.main.GoToPresetFromDevice(): " + err.Error())
	}

	return err
}

func GetPTZFunctionsFromDevice(configurations ptz.GetConfigurationsResponse) ([]string, bool, bool) {
	var functions []string
	canZoom := false
	canPanTilt := false

	if len(configurations.PTZConfiguration) == 0 {
		return functions, canZoom, canPanTilt
	}

	if configurations.PTZConfiguration[0].DefaultAbsolutePantTiltPositionSpace != nil {
		functions = append(functions, "AbsolutePanTiltMove")
		canPanTilt = true
	}
	if configurations.PTZConfiguration[0].DefaultAbsoluteZoomPositionSpace != nil {
		functions = append(functions, "AbsoluteZoomMove")
		canZoom = true
	}
	if configurations.PTZConfiguration[0].DefaultRelativePanTiltTranslationSpace != nil {
		functions = append(functions, "RelativePanTiltMove")
		canPanTilt = true
	}
	if configurations.PTZConfiguration[0].DefaultRelativeZoomTranslationSpace != nil {
		functions = append(functions, "RelativeZoomMove")
		canZoom = true
	}
	if configurations.PTZConfiguration[0].DefaultContinuousPanTiltVelocitySpace != nil {
		functions = append(functions, "ContinuousPanTiltMove")
		canPanTilt = true
	}
	if configurations.PTZConfiguration[0].DefaultContinuousZoomVelocitySpace != nil {
		functions = append(functions, "ContinuousZoomMove")
		canZoom = true
	}
	if configurations.PTZConfiguration[0].DefaultPTZSpeed != nil {
		functions = append(functions, "PTZSpeed")
	}
	if configurations.PTZConfiguration[0].DefaultPTZTimeout != nil {
		functions = append(functions, "PTZTimeout")
	}

	return functions, canZoom, canPanTilt
}

// VerifyOnvifConnection godoc
// @Router /api/camera/onvif/verify [post]
// @ID verify-onvif
// @Security Bearer
// @securityDefinitions.apikey Bearer
// @in header
// @name Authorization
// @Tags onvif
// @Param config body models.OnvifCredentials true "OnvifCredentials"
// @Summary Will verify the ONVIF connectivity.
// @Description Will verify the ONVIF connectivity.
// @Success 200 {object} models.APIResponse
func VerifyOnvifConnection(c *gin.Context) {
	var onvifCredentials models.OnvifCredentials
	err := c.BindJSON(&onvifCredentials)

	if err == nil && onvifCredentials.ONVIFXAddr != "" {

		configuration := &models.Configuration{
			Config: models.Config{
				Capture: models.Capture{
					IPCamera: models.IPCamera{
						ONVIFXAddr:    onvifCredentials.ONVIFXAddr,
						ONVIFUsername: onvifCredentials.ONVIFUsername,
						ONVIFPassword: onvifCredentials.ONVIFPassword,
					},
				},
			},
		}

		cameraConfiguration := configuration.Config.Capture.IPCamera
		device, capabilities, err := ConnectToOnvifDevice(&cameraConfiguration)
		if err == nil {
			// Get token from the first profile
			token, err := GetTokenFromProfile(device, 0)
			if err == nil {
				c.JSON(200, gin.H{
					"device":       device,
					"capabilities": capabilities,
					"token":        token,
				})
			} else {
				c.JSON(400, gin.H{
					"data": "Something went wrong: " + err.Error(),
				})
			}
		} else {
			c.JSON(400, gin.H{
				"data": "Something went wrong: " + err.Error(),
			})
		}
	} else {
		c.JSON(400, gin.H{
			"data": "Something went wrong: " + err.Error(),
		})
	}
}

type ONVIFEvents struct {
	Key       string
	Type      string
	Value     string
	Timestamp int64
}

// Create PullPointSubscription
func CreatePullPointSubscription(dev *onvif.Device) (string, error) {

	// We'll create a subscription to the device
	// This will allow us to receive events from the device
	var createPullPointSubscriptionResponse event.CreatePullPointSubscriptionResponse
	var pullPointAdress string
	var err error

	// For the time being we are just interested in the digital inputs and outputs, therefore
	// we have set the topic to the followin filter.
	terminate := xsd.String("PT60S")
	if dev == nil {
		return pullPointAdress, errors.New("dev is nil, ONVIF was not able to connect to the device")
	}

	resp, err := dev.CallMethod(event.CreatePullPointSubscription{
		InitialTerminationTime: &terminate,

		Filter: &event.FilterType{
			TopicExpression: &event.TopicExpressionType{
				Dialect:    xsd.String("http://www.onvif.org/ver10/tev/topicExpression/ConcreteSet"),
				TopicKinds: "tns1:Device/Trigger//.", // -> This works for Avigilon, Hanwa, Hikvision
				// TopicKinds: "//.", -> This works for Axis, but throws other errors.
			},
		},
	})
	var b2 []byte
	if resp != nil {
		b2, err = io.ReadAll(resp.Body)
		resp.Body.Close() // Ensure the response body is closed
		if err == nil {
			stringBody := string(b2)
			decodedXML, et, err := getXMLNode(stringBody, "CreatePullPointSubscriptionResponse")
			if err != nil {
				log.Debug("onvif.main.CreatePullPointSubscription(): " + err.Error())
			} else {
				if err := decodedXML.DecodeElement(&createPullPointSubscriptionResponse, et); err != nil {
					log.Error("onvif.main.CreatePullPointSubscription(): " + err.Error())
				} else {
					pullPointAdress = string(createPullPointSubscriptionResponse.SubscriptionReference.Address)
				}
			}
		}
	}
	return pullPointAdress, err
}

const (
	wsAddressingNamespace = "http://www.w3.org/2005/08/addressing"
	pullMessagesAction    = "http://www.onvif.org/ver10/events/wsdl/PullPointSubscription/PullMessagesRequest"
	unsubscribeAction     = "http://docs.oasis-open.org/wsn/bw-2/SubscriptionManager/UnsubscribeRequest"
)

// subscriptionAddressingHeader builds the WS-Addressing Action and To headers
// for a request sent to a pull-point subscription. Some devices identify the
// subscription from wsa:To rather than the request URL, and reject requests
// without it as NotAuthorized.
func subscriptionAddressingHeader(subscriptionAddress string, action string) string {
	var address bytes.Buffer
	_ = xml.EscapeText(&address, []byte(subscriptionAddress))
	return `<wsa:Action xmlns:wsa="` + wsAddressingNamespace + `">` + action + `</wsa:Action>` +
		`<wsa:To xmlns:wsa="` + wsAddressingNamespace + `">` + address.String() + `</wsa:To>`
}

func UnsubscribePullPoint(dev *onvif.Device, pullPointAddress string) error {

	// Unsubscribe from the device
	unsubscribe := event.Unsubscribe{}
	requestBody, err := xml.Marshal(unsubscribe)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"component": "onvif",
			"event":     "unsubscribe_request_encoding_failed",
		}).Error("Failed to encode ONVIF unsubscribe request")
		return err
	}

	res, err := dev.SendSoapWithHeader(pullPointAddress, string(requestBody), subscriptionAddressingHeader(pullPointAddress, unsubscribeAction))
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"component": "onvif",
			"event":     "unsubscribe_request_failed",
		}).Error("Failed to send ONVIF unsubscribe request")
		return err
	}
	if res == nil {
		return errors.New("ONVIF unsubscribe returned no response")
	}
	defer res.Body.Close()
	responseBytes, err := io.Copy(io.Discard, res.Body)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"component": "onvif",
			"event":     "unsubscribe_response_read_failed",
		}).Error("Failed to read ONVIF unsubscribe response")
		return err
	}
	log.WithFields(log.Fields{
		"component":      "onvif",
		"event":          "unsubscribe_completed",
		"response_bytes": responseBytes,
		"status_code":    res.StatusCode,
	}).Debug("ONVIF unsubscribe completed")
	return nil
}

// Look for Source of input and output
// Creat a map of the source and the value
// We'll use this map to determine if the value has changed.
// If the value has changed we'll send an event to the frontend.
var inputOutputDeviceMap = make(map[string]*ONVIFEvents)

func GetInputOutputs() ([]ONVIFEvents, error) {
	var eventsArray []ONVIFEvents
	// We have some odd behaviour for inputs: the logical state is set to false even if circuit is closed. However we do see repeated events (looks like heartbeats).
	// We are assuming that if we do not receive an event for 15 seconds the input is inactive, otherwise we set to active.
	for key, value := range inputOutputDeviceMap {
		if time.Now().Unix()-value.Timestamp < 15 && value.Value == "false" {
			value.Value = "true"
		}
		inputOutputDeviceMap[key] = value
		eventsArray = append(eventsArray, *value)
	}
	for _, value := range eventsArray {
		log.WithFields(log.Fields{
			"component":      "onvif",
			"event":          "input_output_state",
			"input_output":   value.Key,
			"state":          value.Value,
			"timestamp_unix": value.Timestamp,
		}).Debug("ONVIF input/output state")
	}
	return eventsArray, nil
}

// ONVIF has a specific profile that requires a subscription to receive events.
// These events can show if an input or output is active or inactive, and also other events.
// For the time being we are only interested in the input and output events, but this can be extended in the future.
func GetEventMessages(dev *onvif.Device, pullPointAddress string) ([]ONVIFEvents, error) {

	var eventsArray []ONVIFEvents
	var err error

	if pullPointAddress != "" {
		// We were able to create a subscription to the device. Now pull some messages from the subscription.
		subscriptionURI := pullPointAddress
		if subscriptionURI == "" {
			log.Error("onvif.main.GetEventMessages(): subscriptionURI is empty")
		} else {
			// Pull message
			pullMessage := event.PullMessages{
				Timeout:      xsd.Duration("PT5S"),
				MessageLimit: 10,
			}
			requestBody, err := xml.Marshal(pullMessage)
			if err != nil {
				log.Error("onvif.main.GetEventMessages(pullMessages): " + err.Error())
				return eventsArray, err
			}
			res, err := dev.SendSoapWithHeader(string(subscriptionURI), string(requestBody), subscriptionAddressingHeader(subscriptionURI, pullMessagesAction))
			if err != nil {
				log.Error("onvif.main.GetEventMessages(pullMessages): " + err.Error())
				return eventsArray, err
			}

			var pullMessagesResponse event.PullMessagesResponse
			if res != nil {
				bs, err := io.ReadAll(res.Body)
				res.Body.Close() // Ensure the response body is closed
				if err == nil {
					stringBody := string(bs)
					decodedXML, et, err := getXMLNode(stringBody, "PullMessagesResponse")
					if err != nil {
						log.Error("onvif.main.GetEventMessages(pullMessages): " + err.Error())
						return eventsArray, err
					} else {
						if err := decodedXML.DecodeElement(&pullMessagesResponse, et); err != nil {
							log.Error("onvif.main.GetEventMessages(pullMessages): " + err.Error())
							return eventsArray, err
						}
					}
				}
			}

			for _, message := range pullMessagesResponse.NotificationMessage {
				log.Debug("onvif.main.GetEventMessages(pullMessages): " + string(message.Topic.TopicKinds))
				//if len(message.Message.Message.Data.SimpleItem) > 0 {
				// log.Debug("onvif.main.GetEventMessages(pullMessages): " + string(message.Message.Message.Data.SimpleItem[0].Name) + " " + string(message.Message.Message.Data.SimpleItem[0].Value))
				//}
				if message.Topic.TopicKinds == "tns1:Device/Trigger/Relay" ||
					message.Topic.TopicKinds == "tns1:Device/tns1:Trigger/tns1:Relay" { // This is for avigilon cameras
					if len(message.Message.Message.Data.SimpleItem) > 0 {
						if message.Message.Message.Data.SimpleItem[0].Name == "LogicalState" ||
							message.Message.Message.Data.SimpleItem[0].Name == "RelayLogicalState" { // On avigilon it's called RelayLogicalState
							key := string(message.Message.Message.Source.SimpleItem[0].Value)
							value := string(message.Message.Message.Data.SimpleItem[0].Value)
							propertyOperation := string(message.Message.Message.PropertyOperation)
							log.WithFields(log.Fields{
								"component":          "onvif",
								"event":              "event_property_output",
								"property":           key,
								"property_operation": propertyOperation,
								"value":              value,
							}).Debug("ONVIF event property output")

							// Depending on the onvif library they might use different values for active and inactive.
							if value == "active" || value == "1" {
								value = "true"
							} else if value == "inactive" || value == "0" {
								value = "false"
							}

							// Check if key exists in map
							// If it does not exist we'll add it to the map otherwise we'll update the value.
							if _, ok := inputOutputDeviceMap[key+"-output"]; !ok {
								inputOutputDeviceMap[key+"-output"] = &ONVIFEvents{
									Key:       key + "-output",
									Type:      "output",
									Value:     value,
									Timestamp: 0,
								}
							} else if propertyOperation == "Changed" {
								inputOutputDeviceMap[key+"-output"].Value = value
								inputOutputDeviceMap[key+"-output"].Timestamp = time.Now().Unix()
							} else if propertyOperation == "Initialized" {
								inputOutputDeviceMap[key+"-output"].Value = value
							}
						}
					}
				} else if message.Topic.TopicKinds == "tns1:Device/Trigger/DigitalInput" ||
					message.Topic.TopicKinds == "tns1:Device/tns1:Trigger/tnssamsung:DigitalInput" { // This is for avigilon's camera
					if len(message.Message.Message.Data.SimpleItem) > 0 {
						if message.Message.Message.Data.SimpleItem[0].Name == "LogicalState" ||
							message.Message.Message.Data.SimpleItem[0].Name == "Level" { // On avigilon it's called level
							key := string(message.Message.Message.Source.SimpleItem[0].Value)
							value := string(message.Message.Message.Data.SimpleItem[0].Value)
							propertyOperation := string(message.Message.Message.PropertyOperation)
							log.WithFields(log.Fields{
								"component":          "onvif",
								"event":              "event_property_input",
								"property":           key,
								"property_operation": propertyOperation,
								"value":              value,
							}).Debug("ONVIF event property input")

							// Depending on the onvif library they might use different values for active and inactive.
							if value == "active" || value == "1" {
								value = "true"
							} else if value == "inactive" || value == "0" {
								value = "false"
							}

							// Check if key exists in map
							// If it does not exist we'll add it to the map otherwise we'll update the value.
							if _, ok := inputOutputDeviceMap[key+"-input"]; !ok {
								inputOutputDeviceMap[key+"-input"] = &ONVIFEvents{
									Key:       key + "-input",
									Type:      "input",
									Value:     value,
									Timestamp: 0,
								}
							} else if propertyOperation == "Changed" {
								inputOutputDeviceMap[key+"-input"].Value = value
								inputOutputDeviceMap[key+"-input"].Timestamp = time.Now().Unix()
							} else if propertyOperation == "Initialized" {
								inputOutputDeviceMap[key+"-input"].Value = value
							}
						}
					}
				}
			}
		}
	}

	eventsArray, _ = GetInputOutputs()
	return eventsArray, err
}

// This method will get the digital inputs from the device.
// But will not give any status information.
func GetDigitalInputs(dev *onvif.Device) (device.GetDigitalInputsResponse, error) {

	// We'll try to receive the relay outputs from the server
	var digitalinputs device.GetDigitalInputsResponse

	var b []byte
	resp, err := dev.CallMethod(deviceio.GetDigitalInputs{})
	if resp != nil {
		b, err = io.ReadAll(resp.Body)
		resp.Body.Close() // Ensure the response body is closed
	}

	if err == nil {
		if err == nil {
			stringBody := string(b)
			decodedXML, et, err := getXMLNode(stringBody, "GetDigitalInputsResponse")
			if err != nil {
				log.Error("onvif.main.GetDigitalInputs(): " + err.Error())
				return digitalinputs, err
			} else {
				if err := decodedXML.DecodeElement(&digitalinputs, et); err != nil {
					log.Debug("onvif.main.GetDigitalInputs(): " + err.Error())
					return digitalinputs, err
				}
			}
		}
	}
	return digitalinputs, err
}

// This method will get the relay outputs from the device.
// But will not give any status information.
func GetRelayOutputs(dev *onvif.Device) (device.GetRelayOutputsResponse, error) {
	// We'll try to receive the relay outputs from the server
	var relayoutputs device.GetRelayOutputsResponse

	// Get the PTZ configurations from the device
	resp, err := dev.CallMethod(device.GetRelayOutputs{})
	var b []byte
	if resp != nil {
		b, err = io.ReadAll(resp.Body)
		resp.Body.Close() // Ensure the response body is closed
	}

	if err == nil {
		stringBody := string(b)
		decodedXML, et, err := getXMLNode(stringBody, "GetRelayOutputsResponse")
		if err != nil {
			log.Error("onvif.main.GetRelayOutputs(): " + err.Error())
			return relayoutputs, err
		} else {
			if err := decodedXML.DecodeElement(&relayoutputs, et); err != nil {
				log.Debug("onvif.main.GetRelayOutputs(): " + err.Error())
				return relayoutputs, err
			}
		}
	}

	return relayoutputs, err
}

func TriggerRelayOutput(dev *onvif.Device, output string) error {
	// Get all outputs
	relayOutputs, err := GetRelayOutputs(dev)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"component": "onvif",
			"event":     "relay_outputs_query_failed",
		}).Error("Failed to query ONVIF relay outputs")
		return err
	}
	if len(relayOutputs.RelayOutputs) == 0 {
		err := errors.New("ONVIF device has no relay outputs")
		log.WithError(err).WithFields(log.Fields{
			"component": "onvif",
			"event":     "relay_output_unavailable",
		}).Error("ONVIF relay output unavailable")
		return err
	}

	// For the moment we expect a single output. Supporting multiple outputs
	// requires matching the requested output against every returned token.
	token := relayOutputs.RelayOutputs[0].Token
	if output != string(token+"-output") {
		err := errors.New("requested ONVIF relay output was not found")
		log.WithError(err).WithFields(log.Fields{
			"component": "onvif",
			"event":     "relay_output_unavailable",
		}).Error("ONVIF relay output unavailable")
		return err
	}

	resp, err := dev.CallMethod(device.SetRelayOutputState{
		RelayOutputToken: token,
		LogicalState:     "active",
	})
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"component": "onvif",
			"event":     "relay_output_request_failed",
		}).Error("Failed to trigger ONVIF relay output")
		return err
	}
	if resp == nil {
		err := errors.New("ONVIF relay request returned no response")
		log.WithError(err).WithFields(log.Fields{
			"component": "onvif",
			"event":     "relay_output_request_failed",
		}).Error("Failed to trigger ONVIF relay output")
		return err
	}
	defer resp.Body.Close()
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		log.WithError(err).WithFields(log.Fields{
			"component": "onvif",
			"event":     "relay_output_response_read_failed",
		}).Warn("Failed to read ONVIF relay response")
		return err
	}
	if resp.StatusCode != 200 {
		err := errors.New("ONVIF relay request was rejected")
		log.WithError(err).WithFields(log.Fields{
			"component":   "onvif",
			"event":       "relay_output_rejected",
			"status_code": resp.StatusCode,
		}).Error("ONVIF relay request rejected")
		return err
	}

	log.WithFields(log.Fields{
		"component": "onvif",
		"event":     "relay_output_triggered",
	}).Info("ONVIF relay output triggered")
	return nil
}

func getXMLNode(xmlBody string, nodeName string) (*xml.Decoder, *xml.StartElement, error) {
	xmlBytes := bytes.NewBufferString(xmlBody)
	decodedXML := xml.NewDecoder(xmlBytes)
	var token xml.Token
	var err error
	for {
		token, err = decodedXML.Token()
		if err != nil {
			break
		}
		switch et := token.(type) {
		case xml.StartElement:
			if et.Name.Local == nodeName {
				return decodedXML, &et, nil
			}
		}
	}

	// Check for authorisation error
	// - The action requested requires authorization and the sender is not authorized
	if strings.Contains(xmlBody, "not authorized") {
		return nil, nil, errors.New("getXMLNode(): not authorized, make sure you have the correct credentials")
	} else {
		return nil, nil, errors.New("getXMLNode(): " + err.Error())
	}
}
