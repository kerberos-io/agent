package onvif

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"io/ioutil"

	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/onvif/media"

	"github.com/kerberos-io/onvif"
	"github.com/kerberos-io/onvif/ptz"
	xsd "github.com/kerberos-io/onvif/xsd/onvif"
)

func HandleONVIFActions(configuration *models.Configuration, communication *models.Communication) {
	log.Log.Debug("HandleONVIFActions: started")

	config := configuration.Config
	for onvifAction := range communication.HandleONVIF {
		var ptzAction models.OnvifActionPTZ
		b, _ := json.Marshal(onvifAction.Payload)
		json.Unmarshal(b, &ptzAction)

		if onvifAction.Action == "ptz" {

			// Do the PTZ shizzle
			device, err := onvif.NewDevice(onvif.DeviceParams{
				Xaddr:    config.Capture.IPCamera.ONVIFXAddr,
				Username: config.Capture.IPCamera.ONVIFUsername,
				Password: config.Capture.IPCamera.ONVIFPassword,
			})

			//services := device.GetServices()
			//fmt.Println(services)

			// Get Profiles
			resp, _ := device.CallMethod(media.GetProfiles{})
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			stringBody := string(b)
			decodedXML, et, errorFunc := getXMLNode(stringBody, "GetProfilesResponse")
			if errorFunc != "" {
			}
			var mProfilesResp media.GetProfilesResponse
			if err := decodedXML.DecodeElement(&mProfilesResp, et); err != nil {
				log.Log.Error("HandleONVIFActions: " + err.Error())
			}
			var ptzToken xsd.ReferenceToken
			for _, profile := range mProfilesResp.Profiles {
				ptzToken = profile.Token
			}

			if err == nil {

				if ptzAction.Center == 1 {
					absoluteVector := xsd.Vector2D{
						X:     0,
						Y:     0,
						Space: "http://www.onvif.org/ver10/tptz/PanTiltSpaces/PositionGenericSpace",
					}

					res, _ := device.CallMethod(ptz.AbsoluteMove{
						ProfileToken: ptzToken,
						Position: xsd.PTZVector{
							PanTilt: absoluteVector,
							Zoom: xsd.Vector1D{
								X:     0,
								Space: "http://www.onvif.org/ver10/tptz/ZoomSpaces/PositionGenericSpace",
							},
						},
					})

					bs, _ := ioutil.ReadAll(res.Body)
					log.Log.Info("HandleONVIFActions: " + string(bs))

				} else {
					distance := 0.1
					panTiltVector := xsd.Vector2D{
						X:     0,
						Y:     0,
						Space: "http://www.onvif.org/ver10/tptz/PanTiltSpaces/TranslationGenericSpace",
					}
					if ptzAction.Left == 1 {
						panTiltVector.X = -1 * distance
					}
					if ptzAction.Right == 1 {
						panTiltVector.X = distance
					}
					if ptzAction.Up == 1 {
						panTiltVector.Y = distance
					}
					if ptzAction.Down == 1 {
						panTiltVector.Y = -1 * distance
					}

					res, _ := device.CallMethod(ptz.RelativeMove{
						ProfileToken: ptzToken,
						Translation: xsd.PTZVector{
							PanTilt: panTiltVector,
							Zoom: xsd.Vector1D{
								X:     0,
								Space: "http://www.onvif.org/ver10/tptz/ZoomSpaces/TranslationGenericSpace",
							},
						},
					})

					bs, _ := ioutil.ReadAll(res.Body)
					log.Log.Info("HandleONVIFActions: " + string(bs))
				}
			}
		}
	}
	log.Log.Debug("HandleONVIFActions: finished")
}

func getXMLNode(xmlBody string, nodeName string) (*xml.Decoder, *xml.StartElement, string) {

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
				return decodedXML, &et, ""
			}
		}
	}
	return nil, nil, "error in NodeName"
}
