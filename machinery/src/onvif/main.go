package onvif

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	onvifc "github.com/cedricve/go-onvif"
	"github.com/gin-gonic/gin"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/onvif/media"

	"github.com/kerberos-io/onvif"
	"github.com/kerberos-io/onvif/ptz"
	xsd "github.com/kerberos-io/onvif/xsd/onvif"
)

func Discover(timeout time.Duration) {
	log.Log.Info("onvif.Discover(): Discovering devices")
	log.Log.Info("Waiting for " + timeout.String())
	devices, err := onvifc.StartDiscovery(timeout)
	if err != nil {
		log.Log.Error("onvif.Discover(): " + err.Error())
	} else {
		for _, device := range devices {
			hostname, _ := device.GetHostname()
			log.Log.Info("onvif.Discover(): " + hostname.Name + " (" + device.XAddr + ")")
		}
		if len(devices) == 0 {
			log.Log.Info("onvif.Discover(): No devices descovered\n")
		}
	}
}

func HandleONVIFActions(configuration *models.Configuration, communication *models.Communication) {
	log.Log.Debug("onvif.HandleONVIFActions(): started")

	for onvifAction := range communication.HandleONVIF {

		// First we'll get the desired PTZ action from the payload
		// We need to know if we need to move left, right, up, down, zoom in, zoom out, center.
		var ptzAction models.OnvifActionPTZ
		b, _ := json.Marshal(onvifAction.Payload)
		json.Unmarshal(b, &ptzAction)

		// Connect to Onvif device
		cameraConfiguration := configuration.Config.Capture.IPCamera
		device, err := ConnectToOnvifDevice(&cameraConfiguration)
		if err == nil {

			// Get token from the first profile
			token, err := GetTokenFromProfile(device, 0)
			if err == nil {

				// Get the configurations from the device
				configurations, err := GetPTZConfigurationsFromDevice(device)

				if err == nil {

					if onvifAction.Action == "absolute-move" {

						// We will move the camera to zero position.
						x := ptzAction.X
						y := ptzAction.Y
						z := ptzAction.Z

						// Check which PTZ Space we need to use
						functions, _, _ := GetPTZFunctionsFromDevice(configurations)

						// Log functions
						log.Log.Debug("onvif.HandleONVIFActions(): functions: " + strings.Join(functions, ", "))

						// Check if we need to use absolute or continuous move
						/*canAbsoluteMove := false
						canContinuousMove := false

						if len(functions) > 0 {
							for _, function := range functions {
								if function == "AbsolutePanTiltMove" || function == "AbsoluteZoomMove" {
									canAbsoluteMove = true
								} else if function == "ContinuousPanTiltMove" || function == "ContinuousZoomMove" {
									canContinuousMove = true
								}
							}
						}*/

						// Ideally we should be able to use the AbsolutePanTiltMove function, but it looks like
						// the current detection through GetPTZFuntionsFromDevice is not working properly. Therefore we will fallback
						// on the ContinuousPanTiltMove function which is more compatible with more cameras.
						err = AbsolutePanTiltMoveFake(device, configurations, token, x, y, z)
						if err != nil {
							log.Log.Debug("onvif.HandleONVIFActions() - AbsolutePanTitleMoveFake: " + err.Error())
						} else {
							log.Log.Info("onvif.HandleONVIFActions() - AbsolutePanTitleMoveFake: successfully moved camera.")
						}

						/*if canAbsoluteMove {
							err = AbsolutePanTiltMove(device, configurations, token, x, y, z)
							if err != nil {
								log.Log.Error("HandleONVIFActions (AbsolutePanTitleMove): " + err.Error())
							}
						} else if canContinuousMove {
							err = AbsolutePanTiltMoveFake(device, configurations, token, x, y, z)
							if err != nil {
								log.Log.Error("HandleONVIFActions (AbsolutePanTitleMoveFake): " + err.Error())
							}
						}*/

					} else if onvifAction.Action == "preset" {

						// Execute the preset
						preset := ptzAction.Preset
						err := GoToPresetFromDevice(device, preset)
						if err != nil {
							log.Log.Debug("onvif.HandleONVIFActions() - GotoPreset: " + err.Error())
						} else {
							log.Log.Info("onvif.HandleONVIFActions() - GotoPreset: successfully moved camera")
						}

					} else if onvifAction.Action == "ptz" {

						if err == nil {

							if ptzAction.Center == 1 {

								// We will move the camera to zero position.
								err := AbsolutePanTiltMove(device, configurations, token, 0, 0, 0)
								if err != nil {
									log.Log.Debug("onvif.HandleONVIFActions() - AbsolutePanTitleMove: " + err.Error())
								} else {
									log.Log.Info("onvif.HandleONVIFActions() - AbsolutePanTitleMove: successfully centered camera")
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
									log.Log.Debug("onvif.HandleONVIFActions() - ContinuousPanTilt: " + err.Error())
								} else {
									log.Log.Info("onvif.HandleONVIFActions() - ContinuousPanTilt: successfully pan tilted camera")
								}
							}
						}
					} else if onvifAction.Action == "zoom" {

						if err == nil {
							zoom := ptzAction.Zoom
							err := ContinuousZoom(device, configurations, token, zoom)
							if err != nil {
								log.Log.Debug("onvif.HandleONVIFActions() - ContinuousZoom: " + err.Error())
							} else {
								log.Log.Info("onvif.HandleONVIFActions() - ContinuousZoom: successfully zoomed camera")
							}
						}
					}
				}
			}
		}
	}
	log.Log.Debug("onvif.HandleONVIFActions(): finished")
}

func ConnectToOnvifDevice(cameraConfiguration *models.IPCamera) (*onvif.Device, error) {
	log.Log.Debug("onvif.ConnectToOnvifDevice(): started")
	device, err := onvif.NewDevice(onvif.DeviceParams{
		Xaddr:    cameraConfiguration.ONVIFXAddr,
		Username: cameraConfiguration.ONVIFUsername,
		Password: cameraConfiguration.ONVIFPassword,
	})
	if err != nil {
		log.Log.Debug("onvif.ConnectToOnvifDevice(): " + err.Error())
	} else {
		log.Log.Info("onvif.ConnectToOnvifDevice(): successfully connected to device")
	}
	log.Log.Debug("onvif.ConnectToOnvifDevice(): finished")
	return device, err
}

func GetTokenFromProfile(device *onvif.Device, profileId int) (xsd.ReferenceToken, error) {
	// We aim to receive a profile token from the server
	var profileToken xsd.ReferenceToken

	// Get Profiles
	resp, err := device.CallMethod(media.GetProfiles{})
	if err == nil {
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err == nil {
			stringBody := string(b)
			decodedXML, et, err := getXMLNode(stringBody, "GetProfilesResponse")
			if err != nil {
				log.Log.Debug("onvif.GetTokenFromProfile(): " + err.Error())
				return profileToken, err
			} else {
				// Decode the profiles from the server
				var mProfilesResp media.GetProfilesResponse
				if err := decodedXML.DecodeElement(&mProfilesResp, et); err != nil {
					log.Log.Debug("onvif.GetTokenFromProfile(): " + err.Error())
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
		resp.Body.Close()
	}

	if err == nil {
		if err == nil {
			stringBody := string(b)
			decodedXML, et, err := getXMLNode(stringBody, "GetConfigurationsResponse")
			if err != nil {
				log.Log.Debug("onvif.GetPTZConfigurationsFromDevice(): " + err.Error())
				return configurations, err
			} else {
				if err := decodedXML.DecodeElement(&configurations, et); err != nil {
					log.Log.Debug("onvif.GetPTZConfigurationsFromDevice(): " + err.Error())
					return configurations, err
				}
			}
		}
	}
	return configurations, err
}

func GetPositionFromDevice(configuration models.Configuration) (xsd.PTZVector, error) {
	var position xsd.PTZVector
	// Connect to Onvif device
	cameraConfiguration := configuration.Config.Capture.IPCamera
	device, err := ConnectToOnvifDevice(&cameraConfiguration)
	if err == nil {

		// Get token from the first profile
		token, err := GetTokenFromProfile(device, 0)
		if err == nil {
			// Get the PTZ configurations from the device
			position, err := GetPosition(device, token)
			if err == nil {
				// float to string
				x := strconv.FormatFloat(position.PanTilt.X, 'f', 6, 64)
				y := strconv.FormatFloat(position.PanTilt.Y, 'f', 6, 64)
				z := strconv.FormatFloat(position.Zoom.X, 'f', 6, 64)
				log.Log.Info("onvif.GetPositionFromDevice(): successfully got position (" + x + ", " + y + ", " + z + ")")
				return position, err
			} else {
				log.Log.Debug("onvif.GetPositionFromDevice(): " + err.Error())
				return position, err
			}
		} else {
			log.Log.Debug("onvif.GetPositionFromDevice(): " + err.Error())
			return position, err
		}
	} else {
		log.Log.Debug("onvif.GetPositionFromDevice(): " + err.Error())
		return position, err
	}
}

func GetPosition(device *onvif.Device, token xsd.ReferenceToken) (xsd.PTZVector, error) {
	// We'll try to receive the PTZ configurations from the server
	var status ptz.GetStatusResponse
	var position xsd.PTZVector

	// Get the PTZ configurations from the device
	resp, err := device.CallMethod(ptz.GetStatus{
		ProfileToken: token,
	})

	var b []byte
	if resp != nil {
		b, err = io.ReadAll(resp.Body)
		resp.Body.Close()
	}

	if err == nil {
		stringBody := string(b)
		decodedXML, et, err := getXMLNode(stringBody, "GetStatusResponse")
		if err != nil {
			log.Log.Error("GetPositionFromDevice: " + err.Error())
			return position, err
		} else {
			if err := decodedXML.DecodeElement(&status, et); err != nil {
				log.Log.Error("GetPositionFromDevice: " + err.Error())
				return position, err
			}
		}
	}
	position = status.PTZStatus.Position
	return position, err
}

func AbsolutePanTiltMove(device *onvif.Device, configuration ptz.GetConfigurationsResponse, token xsd.ReferenceToken, pan float64, tilt float64, zoom float64) error {

	absolutePantiltVector := xsd.Vector2D{
		X:     pan,
		Y:     tilt,
		Space: configuration.PTZConfiguration.DefaultAbsolutePantTiltPositionSpace,
	}

	absoluteZoomVector := xsd.Vector1D{
		X:     zoom,
		Space: configuration.PTZConfiguration.DefaultAbsoluteZoomPositionSpace,
	}

	resp, err := device.CallMethod(ptz.AbsoluteMove{
		ProfileToken: token,
		Position: xsd.PTZVector{
			PanTilt: absolutePantiltVector,
			Zoom:    absoluteZoomVector,
		},
	})

	var b []byte
	if resp != nil {
		b, err = io.ReadAll(resp.Body)
		resp.Body.Close()
	}
	if err != nil {
		log.Log.Error("AbsoluteMove: " + err.Error())
	}
	log.Log.Info("AbsoluteMove: " + string(b))

	return err
}

// This function will simulate the AbsolutePanTiltMove function.
// However the AboslutePanTiltMove function is not working on all cameras.
// So we'll use the ContinuousMove function to simulate the AbsolutePanTiltMove function using the position polling.
func AbsolutePanTiltMoveFake(device *onvif.Device, configuration ptz.GetConfigurationsResponse, token xsd.ReferenceToken, pan float64, tilt float64, zoom float64) error {
	position, err := GetPosition(device, token)
	if position.PanTilt.X >= pan-0.01 && position.PanTilt.X <= pan+0.01 && position.PanTilt.Y >= tilt-0.01 && position.PanTilt.Y <= tilt+0.01 && position.Zoom.X >= zoom-0.01 && position.Zoom.X <= zoom+0.01 {
		log.Log.Debug("AbsolutePanTiltMoveFake: already at position")
	} else {

		// The speed of panning, the higher the faster we'll pan the camera
		// value is a range between 0 and 1.
		speed := 0.6
		wait := 100 * time.Millisecond

		// We'll move quickly to the position (might be inaccurate)
		err = ZoomOutCompletely(device, configuration, token)
		err = PanUntilPosition(device, configuration, token, pan, zoom, speed, wait)
		err = TiltUntilPosition(device, configuration, token, tilt, zoom, speed, wait)

		// Now we'll move a bit slower to make sure we are ok (will be more accurate)
		speed = 0.1
		wait = 200 * time.Millisecond

		err = PanUntilPosition(device, configuration, token, pan, zoom, speed, wait)
		err = TiltUntilPosition(device, configuration, token, tilt, zoom, speed, wait)
		err = ZoomUntilPosition(device, configuration, token, zoom, speed, wait)

		return err
	}
	return err
}

func ZoomOutCompletely(device *onvif.Device, configuration ptz.GetConfigurationsResponse, token xsd.ReferenceToken) error {
	// Zoom out completely!!!
	zoomOut := xsd.Vector1D{
		X:     -1,
		Space: configuration.PTZConfiguration.DefaultContinuousZoomVelocitySpace,
	}
	_, err := device.CallMethod(ptz.ContinuousMove{
		ProfileToken: token,
		Velocity: xsd.PTZSpeedZoom{
			Zoom: zoomOut,
		},
	})
	if err != nil {
		log.Log.Error("ZoomOutCompletely: " + err.Error())
	}

	for {
		position, _ := GetPosition(device, token)
		if position.Zoom.X == 0 {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	_, err = device.CallMethod(ptz.Stop{
		ProfileToken: token,
		Zoom:         true,
	})
	if err != nil {
		log.Log.Error("ZoomOutCompletely: " + err.Error())
	}
	return err
}

func PanUntilPosition(device *onvif.Device, configuration ptz.GetConfigurationsResponse, token xsd.ReferenceToken, pan float64, zoom float64, speed float64, wait time.Duration) error {
	position, err := GetPosition(device, token)

	if position.PanTilt.X >= pan-0.01 && position.PanTilt.X <= pan+0.01 {

	} else {

		// We'll need to determine if we need to move CW or CCW.
		// Check the current position and compare it with the desired position.
		directionX := speed
		if position.PanTilt.X > pan {
			directionX = speed * -1
		}

		panTiltVector := xsd.Vector2D{
			X:     directionX,
			Y:     0,
			Space: configuration.PTZConfiguration.DefaultContinuousPanTiltVelocitySpace,
		}
		resp, err := device.CallMethod(ptz.ContinuousMove{
			ProfileToken: token,
			Velocity: xsd.PTZSpeedPanTilt{
				PanTilt: panTiltVector,
			},
		})

		var b []byte
		if resp != nil {
			b, err = io.ReadAll(resp.Body)
			resp.Body.Close()
		}
		if err != nil {
			log.Log.Error("ContinuousPanTiltMove (Pan): " + err.Error())
		}
		log.Log.Debug("ContinuousPanTiltMove (Pan): " + string(b))

		// While moving we'll check if we reached the desired position.
		// or if we overshot the desired position.

		// Break after 3seconds
		now := time.Now()
		for {
			position, _ := GetPosition(device, token)
			if position.PanTilt.X == -1 || position.PanTilt.X == 1 || (directionX > 0 && position.PanTilt.X >= pan) || (directionX < 0 && position.PanTilt.X <= pan) || (position.PanTilt.X >= pan-0.01 && position.PanTilt.X <= pan+0.01) {
				break
			}
			if time.Since(now) > 3*time.Second {
				break
			}
			time.Sleep(wait)
		}

		_, err = device.CallMethod(ptz.Stop{
			ProfileToken: token,
			PanTilt:      true,
			Zoom:         true,
		})

		if err != nil {
			log.Log.Error("ContinuousPanTiltMove (Pan): " + err.Error())
		}
	}
	return err
}

func TiltUntilPosition(device *onvif.Device, configuration ptz.GetConfigurationsResponse, token xsd.ReferenceToken, tilt float64, zoom float64, speed float64, wait time.Duration) error {
	position, err := GetPosition(device, token)

	if position.PanTilt.Y >= tilt-0.005 && position.PanTilt.Y <= tilt+0.005 {

	} else {

		// We'll need to determine if we need to move CW or CCW.
		// Check the current position and compare it with the desired position.
		directionY := speed
		if position.PanTilt.Y > tilt {
			directionY = speed * -1
		}

		panTiltVector := xsd.Vector2D{
			X:     0,
			Y:     directionY,
			Space: configuration.PTZConfiguration.DefaultContinuousPanTiltVelocitySpace,
		}
		resp, err := device.CallMethod(ptz.ContinuousMove{
			ProfileToken: token,
			Velocity: xsd.PTZSpeedPanTilt{
				PanTilt: panTiltVector,
			},
		})

		var b []byte
		if resp != nil {
			b, err = io.ReadAll(resp.Body)
			resp.Body.Close()
		}

		if err != nil {
			log.Log.Error("ContinuousPanTiltMove (Tilt): " + err.Error())
		}
		log.Log.Debug("ContinuousPanTiltMove (Tilt) " + string(b))

		// While moving we'll check if we reached the desired position.
		// or if we overshot the desired position.

		// Break after 3seconds
		now := time.Now()
		for {
			position, _ := GetPosition(device, token)
			if position.PanTilt.Y == -1 || position.PanTilt.Y == 1 || (directionY > 0 && position.PanTilt.Y >= tilt) || (directionY < 0 && position.PanTilt.Y <= tilt) || (position.PanTilt.Y >= tilt-0.005 && position.PanTilt.Y <= tilt+0.005) {
				break
			}
			if time.Since(now) > 3*time.Second {
				break
			}
			time.Sleep(wait)
		}

		_, err = device.CallMethod(ptz.Stop{
			ProfileToken: token,
			PanTilt:      true,
			Zoom:         true,
		})

		if err != nil {
			log.Log.Error("ContinuousPanTiltMove (Tilt): " + err.Error())
		}
	}
	return err
}

func ZoomUntilPosition(device *onvif.Device, configuration ptz.GetConfigurationsResponse, token xsd.ReferenceToken, zoom float64, speed float64, wait time.Duration) error {
	position, err := GetPosition(device, token)

	if position.Zoom.X >= zoom-0.005 && position.Zoom.X <= zoom+0.005 {

	} else {

		// We'll need to determine if we need to move CW or CCW.
		// Check the current position and compare it with the desired position.
		directionZ := speed
		if position.Zoom.X > zoom {
			directionZ = speed * -1
		}

		zoomVector := xsd.Vector1D{
			X:     directionZ,
			Space: configuration.PTZConfiguration.DefaultContinuousZoomVelocitySpace,
		}
		resp, err := device.CallMethod(ptz.ContinuousMove{
			ProfileToken: token,
			Velocity: xsd.PTZSpeedZoom{
				Zoom: zoomVector,
			},
		})

		var b []byte
		if resp != nil {
			b, err = io.ReadAll(resp.Body)
			resp.Body.Close()
		}
		if err != nil {
			log.Log.Error("ContinuousPanTiltMove (Zoom): " + err.Error())
		}

		log.Log.Debug("ContinuousPanTiltMove (Zoom) " + string(b))

		// While moving we'll check if we reached the desired position.
		// or if we overshot the desired position.

		// Break after 3seconds
		now := time.Now()
		for {
			position, _ := GetPosition(device, token)
			if position.Zoom.X == -1 || position.Zoom.X == 1 || (directionZ > 0 && position.Zoom.X >= zoom) || (directionZ < 0 && position.Zoom.X <= zoom) || (position.Zoom.X >= zoom-0.005 && position.Zoom.X <= zoom+0.005) {
				break
			}
			if time.Since(now) > 3*time.Second {
				break
			}
			time.Sleep(wait)
		}

		_, err = device.CallMethod(ptz.Stop{
			ProfileToken: token,
			PanTilt:      true,
			Zoom:         true,
		})

		if err != nil {
			log.Log.Error("ContinuousPanTiltMove (Zoom): " + err.Error())
		}
	}
	return err
}

func ContinuousPanTilt(device *onvif.Device, configuration ptz.GetConfigurationsResponse, token xsd.ReferenceToken, pan float64, tilt float64) error {

	panTiltVector := xsd.Vector2D{
		X:     pan,
		Y:     tilt,
		Space: configuration.PTZConfiguration.DefaultContinuousPanTiltVelocitySpace,
	}

	resp, err := device.CallMethod(ptz.ContinuousMove{
		ProfileToken: token,
		Velocity: xsd.PTZSpeedPanTilt{
			PanTilt: panTiltVector,
		},
	})

	var b []byte
	if resp != nil {
		b, err = io.ReadAll(resp.Body)
		resp.Body.Close()
	}
	if err != nil {
		log.Log.Error("ContinuousPanTiltMove: " + err.Error())
	}

	log.Log.Debug("ContinuousPanTiltMove: " + string(b))

	time.Sleep(200 * time.Millisecond)

	resp, err = device.CallMethod(ptz.Stop{
		ProfileToken: token,
		PanTilt:      true,
	})

	b = []byte{}
	if resp != nil {
		b, err = io.ReadAll(resp.Body)
		resp.Body.Close()
	}

	if err != nil {
		log.Log.Error("ContinuousPanTiltMove: " + err.Error())
	}

	return err
}

func ContinuousZoom(device *onvif.Device, configuration ptz.GetConfigurationsResponse, token xsd.ReferenceToken, zoom float64) error {

	zoomVector := xsd.Vector1D{
		X:     zoom,
		Space: configuration.PTZConfiguration.DefaultContinuousZoomVelocitySpace,
	}

	resp, err := device.CallMethod(ptz.ContinuousMove{
		ProfileToken: token,
		Velocity: xsd.PTZSpeedZoom{
			Zoom: zoomVector,
		},
	})

	var b []byte
	if resp != nil {
		b, err = io.ReadAll(resp.Body)
		resp.Body.Close()
	}
	if err != nil {
		log.Log.Error("ContinuousPanTiltZoom: " + err.Error())
	}

	log.Log.Debug("ContinuousPanTiltZoom: " + string(b))

	time.Sleep(500 * time.Millisecond)

	resp, err = device.CallMethod(ptz.Stop{
		ProfileToken: token,
		Zoom:         true,
	})

	b = []byte{}
	if resp != nil {
		b, err = io.ReadAll(resp.Body)
		resp.Body.Close()
	}
	if err != nil {
		log.Log.Error("ContinuousPanTiltZoom: " + err.Error())
	}

	return err
}

func GetCapabilitiesFromDevice(device *onvif.Device) []string {
	var capabilities []string
	services := device.GetServices()
	for key, _ := range services {
		log.Log.Debug("GetCapabilitiesFromDevice: has key: " + key)
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
			resp.Body.Close()
		}

		if err == nil {
			stringBody := string(b)
			decodedXML, et, err := getXMLNode(stringBody, "GetPresetsResponse")
			if err != nil {
				log.Log.Error("GetPresetsFromDevice: " + err.Error())
				return presets, err
			} else {
				if err := decodedXML.DecodeElement(&presetsResponse, et); err != nil {
					log.Log.Error("GetPresetsFromDevice: " + err.Error())
					return presets, err
				}

				for _, preset := range presetsResponse.Preset {
					p := models.OnvifActionPreset{
						Name:  string(preset.Name),
						Token: string(preset.Token),
					}

					presets = append(presets, p)
				}

				return presets, err
			}
		} else {
			log.Log.Error("GetPresetsFromDevice: " + err.Error())
		}
	} else {
		log.Log.Error("GetPresetsFromDevice: " + err.Error())
	}

	return presets, err
}

func GoToPresetFromDevice(device *onvif.Device, presetName string) error {
	var goToPresetResponse ptz.GotoPresetResponse

	// Get token from the first profile
	token, err := GetTokenFromProfile(device, 0)
	if err == nil {

		resp, err := device.CallMethod(ptz.GotoPreset{
			ProfileToken: token,
			PresetToken:  xsd.ReferenceToken(presetName),
		})
		var b []byte
		if resp != nil {
			b, err = io.ReadAll(resp.Body)
			resp.Body.Close()
		}
		if err == nil {
			stringBody := string(b)
			decodedXML, et, err := getXMLNode(stringBody, "GotoPresetResponses")
			if err != nil {
				log.Log.Error("GoToPresetFromDevice: " + err.Error())
				return err
			} else {
				if err := decodedXML.DecodeElement(&goToPresetResponse, et); err != nil {
					log.Log.Error("GoToPresetFromDevice: " + err.Error())
					return err
				}
				return err
			}
		} else {
			log.Log.Error("GoToPresetFromDevice: " + err.Error())
		}
	} else {
		log.Log.Error("GoToPresetFromDevice: " + err.Error())
	}

	return err
}

func GetPTZFunctionsFromDevice(configurations ptz.GetConfigurationsResponse) ([]string, bool, bool) {
	var functions []string
	canZoom := false
	canPanTilt := false

	if configurations.PTZConfiguration.DefaultAbsolutePantTiltPositionSpace != "" {
		functions = append(functions, "AbsolutePanTiltMove")
		canPanTilt = true
	}
	if configurations.PTZConfiguration.DefaultAbsoluteZoomPositionSpace != "" {
		functions = append(functions, "AbsoluteZoomMove")
		canZoom = true
	}
	if configurations.PTZConfiguration.DefaultRelativePanTiltTranslationSpace != "" {
		functions = append(functions, "RelativePanTiltMove")
		canPanTilt = true
	}
	if configurations.PTZConfiguration.DefaultRelativeZoomTranslationSpace != "" {
		functions = append(functions, "RelativeZoomMove")
		canZoom = true
	}
	if configurations.PTZConfiguration.DefaultContinuousPanTiltVelocitySpace != "" {
		functions = append(functions, "ContinuousPanTiltMove")
		canPanTilt = true
	}
	if configurations.PTZConfiguration.DefaultContinuousZoomVelocitySpace != "" {
		functions = append(functions, "ContinuousZoomMove")
		canZoom = true
	}
	if configurations.PTZConfiguration.DefaultPTZSpeed != nil {
		functions = append(functions, "PTZSpeed")
	}
	if configurations.PTZConfiguration.DefaultPTZTimeout != "" {
		functions = append(functions, "PTZTimeout")
	}

	return functions, canZoom, canPanTilt
}

// VerifyOnvifConnection godoc
// @Router /api/onvif/verify [post]
// @ID verify-onvif
// @Security Bearer
// @securityDefinitions.apikey Bearer
// @in header
// @name Authorization
// @Tags general
// @Param cameraConfig body models.IPCamera true "Camera Config"
// @Summary Will verify the ONVIF connectivity.
// @Description Will verify the ONVIF connectivity.
// @Success 200 {object} models.APIResponse
func VerifyOnvifConnection(c *gin.Context) {
	var cameraConfig models.IPCamera
	err := c.BindJSON(&cameraConfig)
	if err == nil {
		device, err := ConnectToOnvifDevice(&cameraConfig)
		if err == nil {
			// Get the list of configurations
			configurations, err := GetPTZConfigurationsFromDevice(device)
			if err == nil {

				// Check if can zoom and/or pan/tilt is supported
				ptzFunctions, canZoom, canPanTilt := GetPTZFunctionsFromDevice(configurations)
				c.JSON(200, models.APIResponse{
					Data:         device,
					PTZFunctions: ptzFunctions,
					CanZoom:      canZoom,
					CanPanTilt:   canPanTilt,
				})
			} else {
				c.JSON(400, models.APIResponse{
					Message: "Something went wrong while getting the configurations " + err.Error(),
				})
			}
		} else {
			c.JSON(400, models.APIResponse{
				Message: "Something went wrong while verifying the ONVIF connection " + err.Error(),
			})
		}
	} else {
		c.JSON(400, models.APIResponse{
			Message: "Something went wrong while receiving the config " + err.Error(),
		})
	}
}

func getXMLNode(xmlBody string, nodeName string) (*xml.Decoder, *xml.StartElement, error) {
	xmlBytes := bytes.NewBufferString(xmlBody)
	decodedXML := xml.NewDecoder(xmlBytes)
	for {
		token, err := decodedXML.Token()
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
	return nil, nil, errors.New("error in NodeName - username and password might be wrong")
}
