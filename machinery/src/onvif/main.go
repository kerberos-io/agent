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

	"github.com/gin-gonic/gin"
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
						err := AbsolutePanTiltMove(device, configurations, token, x, y, z)
						if err != nil {
							log.Log.Error("HandleONVIFActions (AbsolutePanTitleMove): " + err.Error())
						}

					} else if onvifAction.Action == "ptz" {

						if err == nil {

							if ptzAction.Center == 1 {

								// We will move the camera to zero position.
								err := AbsolutePanTiltMove(device, configurations, token, 0, 0, 0)
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

func ConnectToOnvifDevice(cameraConfiguration *models.IPCamera) (*onvif.Device, error) {
	log.Log.Debug("ConnectToOnvifDevice: started")

	device, err := onvif.NewDevice(onvif.DeviceParams{
		Xaddr:    cameraConfiguration.ONVIFXAddr,
		Username: cameraConfiguration.ONVIFUsername,
		Password: cameraConfiguration.ONVIFPassword,
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

func GetPTZConfigurationsFromDevice(device *onvif.Device) (ptz.GetConfigurationsResponse, error) {
	// We'll try to receive the PTZ configurations from the server
	var configurations ptz.GetConfigurationsResponse

	// Get the PTZ configurations from the device
	resp, err := device.CallMethod(ptz.GetConfigurations{})
	if err == nil {
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err == nil {
			stringBody := string(b)
			decodedXML, et, err := getXMLNode(stringBody, "GetConfigurationsResponse")
			if err != nil {
				log.Log.Error("GetPTZConfigurationsFromDevice: " + err.Error())
				return configurations, err
			} else {
				if err := decodedXML.DecodeElement(&configurations, et); err != nil {
					log.Log.Error("GetPTZConfigurationsFromDevice: " + err.Error())
					return configurations, err
				}
			}
		}
	}
	return configurations, err
}

func GetPositionFromDevice(configuration models.Configuration) (xsd.PTZVector, error) {

	// We'll try to receive the PTZ configurations from the server
	var status ptz.GetStatusResponse
	var position xsd.PTZVector

	// Connect to Onvif device
	cameraConfiguration := configuration.Config.Capture.IPCamera
	device, err := ConnectToOnvifDevice(&cameraConfiguration)
	if err == nil {

		// Get token from the first profile
		token, err := GetTokenFromProfile(device, 0)
		if err == nil {

			// Get the PTZ configurations from the device
			resp, err := device.CallMethod(ptz.GetStatus{
				ProfileToken: token,
			})

			if err == nil {
				defer resp.Body.Close()
				b, err := io.ReadAll(resp.Body)
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
			}
			position = status.PTZStatus.Position
			return position, err
		} else {
			log.Log.Error("GetPositionFromDevice: " + err.Error())
			return position, err
		}
	} else {
		log.Log.Error("GetPositionFromDevice: " + err.Error())
		return position, err
	}
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

	res, err := device.CallMethod(ptz.AbsoluteMove{
		ProfileToken: token,
		Position: xsd.PTZVector{
			PanTilt: absolutePantiltVector,
			Zoom:    absoluteZoomVector,
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
// @Tags config
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
