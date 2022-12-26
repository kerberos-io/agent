package onvif

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"io/ioutil"
	"strings"
	"time"

	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/onvif/media"

	"github.com/kerberos-io/onvif"
	"github.com/kerberos-io/onvif/ptz"
	xsd "github.com/kerberos-io/onvif/xsd/onvif"
)

func HandleONVIFActions(configuration *models.Configuration, communication *models.Communication) {
	log.Log.Debug("HandleONVIFActions: started")

	for onvifAction := range communication.HandleONVIF {

		// First we'll get the desired PTZ action from the payload
		// We need to know if we need to move left, right, up, down, zoom in, zoom out, center.
		var ptzAction models.OnvifActionPTZ
		b, _ := json.Marshal(onvifAction.Payload)
		json.Unmarshal(b, &ptzAction)

		// Connect to Onvif device
		device, err := ConnectToOnvifDevice(configuration)
		if err == nil {

			// Get token from the first profile
			token, err := GetTokenFromProfile(device, 0)
			if err == nil {

				// Get the configurations from the device
				configurations, err := GetConfigurationsFromDevice(device)

				if err == nil {

					if onvifAction.Action == "ptz" {

						if err == nil {

							if ptzAction.Center == 1 {

								// We will move the camera to zero position.
								err := AbsolutePanTiltMove(device, configurations, token, 0, 0)
								if err != nil {
									log.Log.Error("HandleONVIFActions (AbsolutePanTitleMove): " + err.Error())
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
									log.Log.Error("HandleONVIFActions (ContinuousPanTilt): " + err.Error())
								}
							}
						}
					} else if onvifAction.Action == "zoom" {

						if err == nil {
							zoom := ptzAction.Zoom
							err := ContinuousZoom(device, configurations, token, zoom)
							if err != nil {
								log.Log.Error("HandleONVIFActions (ContinuousZoom): " + err.Error())
							}
						}
					}
				}
			}
		}
	}
	log.Log.Debug("HandleONVIFActions: finished")
}

func ConnectToOnvifDevice(configuration *models.Configuration) (*onvif.Device, error) {
	log.Log.Debug("ConnectToOnvifDevice: started")

	config := configuration.Config

	// Get the capabilities of the ONVIF device
	device, err := onvif.NewDevice(onvif.DeviceParams{
		Xaddr:    config.Capture.IPCamera.ONVIFXAddr,
		Username: config.Capture.IPCamera.ONVIFUsername,
		Password: config.Capture.IPCamera.ONVIFPassword,
	})

	if err != nil {
		log.Log.Error("ConnectToOnvifDevice: " + err.Error())
	}

	log.Log.Debug("ConnectToOnvifDevice: finished")
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
				log.Log.Error("GetTokenFromProfile: " + err.Error())
				return profileToken, err
			} else {
				// Decode the profiles from the server
				var mProfilesResp media.GetProfilesResponse
				if err := decodedXML.DecodeElement(&mProfilesResp, et); err != nil {
					log.Log.Error("GetTokenFromProfile: " + err.Error())
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

func GetConfigurationsFromDevice(device *onvif.Device) (ptz.GetConfigurationsResponse, error) {
	// We'll try to receive the PTZ configurations from the server
	var configurations ptz.GetConfigurationsResponse

	// Get the configurations from the device
	resp, err := device.CallMethod(ptz.GetConfigurations{})
	if err == nil {
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err == nil {
			stringBody := string(b)
			decodedXML, et, err := getXMLNode(stringBody, "GetConfigurationsResponse")
			if err != nil {
				log.Log.Error("GetConfigurationsFromDevice: " + err.Error())
				return configurations, err
			} else {
				if err := decodedXML.DecodeElement(&configurations, et); err != nil {
					log.Log.Error("GetConfigurationsFromDevice: " + err.Error())
					return configurations, err
				}
			}
		}
	}
	return configurations, err
}

func AbsolutePanTiltMove(device *onvif.Device, configuration ptz.GetConfigurationsResponse, token xsd.ReferenceToken, pan float32, tilt float32) error {

	absoluteVector := xsd.Vector2D{
		X:     float64(pan),
		Y:     float64(tilt),
		Space: configuration.PTZConfiguration.DefaultAbsolutePantTiltPositionSpace,
	}

	res, err := device.CallMethod(ptz.AbsoluteMove{
		ProfileToken: token,
		Position: xsd.PTZVector{
			PanTilt: absoluteVector,
		},
	})

	if err != nil {
		log.Log.Error("AbsoluteMove: " + err.Error())
	}

	bs, _ := ioutil.ReadAll(res.Body)
	log.Log.Debug("AbsoluteMove: " + string(bs))

	return err
}

func ContinuousPanTilt(device *onvif.Device, configuration ptz.GetConfigurationsResponse, token xsd.ReferenceToken, pan float64, tilt float64) error {

	panTiltVector := xsd.Vector2D{
		X:     pan,
		Y:     tilt,
		Space: configuration.PTZConfiguration.DefaultContinuousPanTiltVelocitySpace,
	}

	res, err := device.CallMethod(ptz.ContinuousMove{
		ProfileToken: token,
		Velocity: xsd.PTZSpeedPanTilt{
			PanTilt: panTiltVector,
		},
	})

	if err != nil {
		log.Log.Error("ContinuousPanTiltMove: " + err.Error())
	}

	bs, _ := ioutil.ReadAll(res.Body)
	log.Log.Debug("ContinuousPanTiltMove: " + string(bs))

	time.Sleep(500 * time.Millisecond)

	res, errStop := device.CallMethod(ptz.Stop{
		ProfileToken: token,
		PanTilt:      true,
	})

	if errStop != nil {
		log.Log.Error("ContinuousPanTiltMove: " + errStop.Error())
	}

	if errStop == nil {
		return err
	} else {
		return errStop
	}
}

func ContinuousZoom(device *onvif.Device, configuration ptz.GetConfigurationsResponse, token xsd.ReferenceToken, zoom float64) error {

	zoomVector := xsd.Vector1D{
		X:     zoom,
		Space: configuration.PTZConfiguration.DefaultContinuousZoomVelocitySpace,
	}

	res, err := device.CallMethod(ptz.ContinuousMove{
		ProfileToken: token,
		Velocity: xsd.PTZSpeedZoom{
			Zoom: zoomVector,
		},
	})

	if err != nil {
		log.Log.Error("ContinuousPanTiltZoom: " + err.Error())
	}

	bs, _ := ioutil.ReadAll(res.Body)
	log.Log.Debug("ContinuousPanTiltZoom: " + string(bs))

	time.Sleep(500 * time.Millisecond)

	res, errStop := device.CallMethod(ptz.Stop{
		ProfileToken: token,
		Zoom:         true,
	})

	if errStop != nil {
		log.Log.Error("ContinuousPanTiltZoom: " + errStop.Error())
	}

	if errStop == nil {
		return err
	} else {
		return errStop
	}
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
