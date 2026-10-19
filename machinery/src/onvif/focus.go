package onvif

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"slices"
	"time"

	"github.com/kerberos-io/onvif"
	"github.com/kerberos-io/onvif/imaging"
	"github.com/kerberos-io/onvif/media"
	xsdonvif "github.com/kerberos-io/onvif/xsd/onvif"
	log "github.com/sirupsen/logrus"
)

// Focus control goes through the ONVIF imaging service. The request types of
// the onvif library serialise every field (all three move types, every imaging
// setting), which cameras reject or which would overwrite unrelated imaging
// settings. The types below only carry the fields we want to send. Schema
// elements use the "onvif" prefix, which the library declares on the envelope
// as http://www.onvif.org/ver10/schema.

const (
	focusModeAuto   = "AUTO"
	focusModeManual = "MANUAL"

	// Same move duration as ContinuousZoom.
	continuousFocusDuration = 500 * time.Millisecond
	// Step for cameras that only support relative focus moves.
	relativeFocusDistance = 0.1
	// Step at full speed for cameras that only support absolute focus moves
	// (e.g. Axis); scaled by the requested speed.
	absoluteFocusStep = 0.1
	// Time to let the camera settle in MANUAL before re-enabling AUTO.
	autoFocusRetriggerDelay = 300 * time.Millisecond
)

type imagingMoveRequest struct {
	XMLName          xml.Name                `xml:"timg:Move"`
	VideoSourceToken xsdonvif.ReferenceToken `xml:"timg:VideoSourceToken"`
	Focus            focusMove               `xml:"timg:Focus"`
}

type focusMove struct {
	Absolute   *absoluteFocusMove   `xml:"onvif:Absolute,omitempty"`
	Relative   *relativeFocusMove   `xml:"onvif:Relative,omitempty"`
	Continuous *continuousFocusMove `xml:"onvif:Continuous,omitempty"`
}

type absoluteFocusMove struct {
	Position float64 `xml:"onvif:Position"`
}

type relativeFocusMove struct {
	Distance float64 `xml:"onvif:Distance"`
	Speed    float64 `xml:"onvif:Speed,omitempty"`
}

type continuousFocusMove struct {
	Speed float64 `xml:"onvif:Speed"`
}

type imagingStopRequest struct {
	XMLName          xml.Name                `xml:"timg:Stop"`
	VideoSourceToken xsdonvif.ReferenceToken `xml:"timg:VideoSourceToken"`
}

type imagingGetSettingsRequest struct {
	XMLName          xml.Name                `xml:"timg:GetImagingSettings"`
	VideoSourceToken xsdonvif.ReferenceToken `xml:"timg:VideoSourceToken"`
}

type imagingGetSettingsResponse struct {
	ImagingSettings struct {
		Brightness *float64 `xml:"Brightness"`
		Focus      struct {
			AutoFocusMode string `xml:"AutoFocusMode"`
		} `xml:"Focus"`
	} `xml:"ImagingSettings"`
}

type imagingSetFocusModeRequest struct {
	XMLName          xml.Name                `xml:"timg:SetImagingSettings"`
	VideoSourceToken xsdonvif.ReferenceToken `xml:"timg:VideoSourceToken"`
	ImagingSettings  struct {
		Brightness *float64 `xml:"onvif:Brightness,omitempty"`
		Focus      struct {
			AutoFocusMode string `xml:"onvif:AutoFocusMode"`
		} `xml:"onvif:Focus"`
	} `xml:"timg:ImagingSettings"`
}

type imagingGetOptionsRequest struct {
	XMLName          xml.Name                `xml:"timg:GetOptions"`
	VideoSourceToken xsdonvif.ReferenceToken `xml:"timg:VideoSourceToken"`
}

type imagingGetOptionsResponse struct {
	ImagingOptions struct {
		Focus struct {
			AutoFocusModes []string `xml:"AutoFocusModes"`
		} `xml:"Focus"`
	} `xml:"ImagingOptions"`
}

type imagingGetMoveOptionsRequest struct {
	XMLName          xml.Name                `xml:"timg:GetMoveOptions"`
	VideoSourceToken xsdonvif.ReferenceToken `xml:"timg:VideoSourceToken"`
}

type imagingGetMoveOptionsResponse struct {
	MoveOptions struct {
		Absolute *struct {
			Position struct {
				Min float64 `xml:"Min"`
				Max float64 `xml:"Max"`
			} `xml:"Position"`
		} `xml:"Absolute"`
		Relative   *struct{} `xml:"Relative"`
		Continuous *struct{} `xml:"Continuous"`
	} `xml:"MoveOptions"`
}

type imagingGetStatusRequest struct {
	XMLName          xml.Name                `xml:"timg:GetStatus"`
	VideoSourceToken xsdonvif.ReferenceToken `xml:"timg:VideoSourceToken"`
}

type imagingGetStatusResponse struct {
	Status struct {
		FocusStatus20 *struct {
			Position *float64 `xml:"Position"`
		} `xml:"FocusStatus20"`
		FocusStatus *struct {
			Position *float64 `xml:"Position"`
		} `xml:"FocusStatus"`
	} `xml:"Status"`
}

// focusMoveOptions lists the focus move types a camera supports.
type focusMoveOptions struct {
	Continuous  bool
	Relative    bool
	Absolute    bool
	AbsoluteMin float64
	AbsoluteMax float64
}

// allFocusMoveOptions is used when a camera does not report its move options:
// every move type is tried in turn.
var allFocusMoveOptions = focusMoveOptions{Continuous: true, Relative: true, Absolute: true, AbsoluteMin: 0, AbsoluteMax: 1}

// focusController abstracts the imaging calls so the focus logic can be tested
// without a camera.
type focusController interface {
	FocusMode() (string, error)
	FocusModes() ([]string, error)
	SetFocusMode(mode string) error
	MoveOptions() (focusMoveOptions, error)
	FocusPosition() (float64, error)
	MoveFocus(move focusMove) error
	StopFocus() error
}

type onvifFocusClient struct {
	device   *onvif.Device
	endpoint string
	token    xsdonvif.ReferenceToken
}

func newONVIFFocusClient(device *onvif.Device) (*onvifFocusClient, error) {
	endpoint, err := device.GetEndpointByRequestStruct(&imaging.Move{})
	if err != nil {
		return nil, errors.New("camera does not expose the ONVIF imaging service: " + err.Error())
	}
	token, err := GetVideoSourceTokenFromProfile(device, 0)
	if err != nil {
		return nil, err
	}
	return &onvifFocusClient{device: device, endpoint: endpoint, token: token}, nil
}

var soapFaultReason = regexp.MustCompile(`<(?:[\w.-]+:)?Text[^>]*>([^<]+)</`)

// call sends an imaging request and returns the response body. A rejected
// request returns an error carrying the camera's SOAP fault reason.
func (client *onvifFocusClient) call(operation string, request interface{}) ([]byte, error) {
	body, err := xml.Marshal(request)
	if err != nil {
		return nil, err
	}
	resp, err := client.device.SendSoap(client.endpoint, string(body))
	if resp == nil {
		if err == nil {
			err = errors.New("no response")
		}
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	b, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		reason := http.StatusText(resp.StatusCode)
		if m := soapFaultReason.FindSubmatch(b); m != nil {
			reason = string(m[1])
		}
		return nil, fmt.Errorf("%s: camera returned HTTP %d: %s", operation, resp.StatusCode, reason)
	}
	if readErr != nil {
		return nil, fmt.Errorf("%s: %w", operation, readErr)
	}
	return b, nil
}

func (client *onvifFocusClient) query(operation string, request interface{}, responseNode string, response interface{}) error {
	b, err := client.call(operation, request)
	if err != nil {
		return err
	}
	decodedXML, et, err := getXMLNode(string(b), responseNode)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	if err := decodedXML.DecodeElement(response, et); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

func (client *onvifFocusClient) imagingSettings() (imagingGetSettingsResponse, error) {
	var settings imagingGetSettingsResponse
	err := client.query("get_imaging_settings", imagingGetSettingsRequest{VideoSourceToken: client.token}, "GetImagingSettingsResponse", &settings)
	return settings, err
}

func (client *onvifFocusClient) FocusMode() (string, error) {
	settings, err := client.imagingSettings()
	return settings.ImagingSettings.Focus.AutoFocusMode, err
}

func (client *onvifFocusClient) SetFocusMode(mode string) error {
	request := imagingSetFocusModeRequest{VideoSourceToken: client.token}
	request.ImagingSettings.Focus.AutoFocusMode = mode
	_, err := client.call("set_focus_mode", request)
	if err == nil {
		return nil
	}

	// Some cameras (seen on Axis) fail on a SetImagingSettings that only
	// carries Focus. Retry with the current brightness, which leaves it as is.
	settings, settingsErr := client.imagingSettings()
	if settingsErr != nil || settings.ImagingSettings.Brightness == nil {
		return err
	}
	request.ImagingSettings.Brightness = settings.ImagingSettings.Brightness
	if _, retryErr := client.call("set_focus_mode", request); retryErr != nil {
		return errors.Join(err, retryErr)
	}
	return nil
}

func (client *onvifFocusClient) FocusModes() ([]string, error) {
	var response imagingGetOptionsResponse
	if err := client.query("get_imaging_options", imagingGetOptionsRequest{VideoSourceToken: client.token}, "GetOptionsResponse", &response); err != nil {
		return nil, err
	}
	return response.ImagingOptions.Focus.AutoFocusModes, nil
}

func (client *onvifFocusClient) MoveOptions() (focusMoveOptions, error) {
	var response imagingGetMoveOptionsResponse
	if err := client.query("get_focus_move_options", imagingGetMoveOptionsRequest{VideoSourceToken: client.token}, "GetMoveOptionsResponse", &response); err != nil {
		return focusMoveOptions{}, err
	}
	moveOptions := response.MoveOptions
	options := focusMoveOptions{
		Continuous:  moveOptions.Continuous != nil,
		Relative:    moveOptions.Relative != nil,
		Absolute:    moveOptions.Absolute != nil,
		AbsoluteMin: 0,
		AbsoluteMax: 1,
	}
	if moveOptions.Absolute != nil && moveOptions.Absolute.Position.Max > moveOptions.Absolute.Position.Min {
		options.AbsoluteMin = moveOptions.Absolute.Position.Min
		options.AbsoluteMax = moveOptions.Absolute.Position.Max
	}
	return options, nil
}

func (client *onvifFocusClient) FocusPosition() (float64, error) {
	var response imagingGetStatusResponse
	if err := client.query("get_focus_status", imagingGetStatusRequest{VideoSourceToken: client.token}, "GetStatusResponse", &response); err != nil {
		return 0, err
	}
	status := response.Status
	if status.FocusStatus20 != nil && status.FocusStatus20.Position != nil {
		return *status.FocusStatus20.Position, nil
	}
	if status.FocusStatus != nil && status.FocusStatus.Position != nil {
		return *status.FocusStatus.Position, nil
	}
	return 0, errors.New("get_focus_status: camera did not report a focus position")
}

func (client *onvifFocusClient) MoveFocus(move focusMove) error {
	_, err := client.call("focus_move", imagingMoveRequest{VideoSourceToken: client.token, Focus: move})
	return err
}

func (client *onvifFocusClient) StopFocus() error {
	_, err := client.call("focus_stop", imagingStopRequest{VideoSourceToken: client.token})
	return err
}

// ContinuousFocus moves the focus motor a short step, the focus equivalent of
// ContinuousZoom. Speed is in [-1, 1]: positive focuses near, negative focuses
// far. Cameras with continuous focus move for a moment and stop; cameras with
// only relative or absolute focus (e.g. Axis) step by an amount scaled to the
// speed. The camera is switched to MANUAL focus first, because cameras ignore
// manual moves while autofocus is active; use AutoFocus to hand control back.
func ContinuousFocus(device *onvif.Device, speed float64) error {
	client, err := newONVIFFocusClient(device)
	if err != nil {
		return err
	}
	return continuousFocus(client, speed, continuousFocusDuration)
}

// AutoFocus triggers the camera's native ONVIF autofocus. Setting AUTO while
// already in AUTO is a no-op on most cameras, so we toggle through MANUAL to
// force a fresh focus cycle.
func AutoFocus(device *onvif.Device) error {
	client, err := newONVIFFocusClient(device)
	if err != nil {
		return err
	}
	return autoFocus(client, autoFocusRetriggerDelay)
}

func continuousFocus(client focusController, speed float64, duration time.Duration) error {
	speed = math.Max(-1, math.Min(1, speed))
	if speed == 0 {
		return client.StopFocus()
	}

	if err := ensureFocusMode(client, focusModeManual); err != nil {
		// Some cameras have no focus mode setting but still accept moves.
		log.Debug("onvif.continuousFocus(): could not switch to manual focus: " + err.Error())
	}

	options, err := client.MoveOptions()
	if err != nil {
		log.Debug("onvif.continuousFocus(): could not read focus move options, trying all move types: " + err.Error())
		options = allFocusMoveOptions
	}

	var moveErrs []error
	if options.Continuous {
		err := client.MoveFocus(focusMove{Continuous: &continuousFocusMove{Speed: speed}})
		if err == nil {
			time.Sleep(duration)
			return client.StopFocus()
		}
		moveErrs = append(moveErrs, err)
	}
	if options.Relative {
		// A single relative step needs no stop.
		err := client.MoveFocus(focusMove{Relative: &relativeFocusMove{
			Distance: math.Copysign(relativeFocusDistance, speed),
			Speed:    math.Abs(speed),
		}})
		if err == nil {
			return nil
		}
		moveErrs = append(moveErrs, err)
	}
	if options.Absolute {
		position, err := client.FocusPosition()
		if err == nil {
			target := math.Max(options.AbsoluteMin, math.Min(options.AbsoluteMax, position+speed*absoluteFocusStep))
			err = client.MoveFocus(focusMove{Absolute: &absoluteFocusMove{Position: target}})
		}
		if err == nil {
			return nil
		}
		moveErrs = append(moveErrs, err)
	}

	if len(moveErrs) == 0 {
		return errors.New("camera does not support any focus move")
	}
	return errors.Join(moveErrs...)
}

func ensureFocusMode(client focusController, mode string) error {
	current, err := client.FocusMode()
	if err == nil && current == mode {
		return nil
	}
	return client.SetFocusMode(mode)
}

var errNoNativeAutoFocus = errors.New("camera does not support native ONVIF autofocus")

func autoFocus(client focusController, delay time.Duration) error {
	// Some cameras (seen on Dahua) only advertise MANUAL, yet accept a switch
	// to AUTO and silently ignore it.
	if modes, err := client.FocusModes(); err == nil && !slices.Contains(modes, focusModeAuto) {
		return errNoNativeAutoFocus
	}

	mode, err := client.FocusMode()
	if err == nil && mode == focusModeAuto {
		if err := client.SetFocusMode(focusModeManual); err != nil {
			return err
		}
		time.Sleep(delay)
	}
	if err := client.SetFocusMode(focusModeAuto); err != nil {
		return err
	}

	if mode, err := client.FocusMode(); err == nil && mode != focusModeAuto {
		return fmt.Errorf("%w: camera accepted AUTO but reports %s", errNoNativeAutoFocus, mode)
	}
	return nil
}

// GetVideoSourceTokenFromProfile returns the video source token of a media
// profile; the imaging service is addressed by video source, not by profile.
func GetVideoSourceTokenFromProfile(device *onvif.Device, profileId int) (xsdonvif.ReferenceToken, error) {
	var sourceToken xsdonvif.ReferenceToken

	resp, err := device.CallMethod(media.GetProfiles{})
	if err != nil {
		if resp != nil {
			resp.Body.Close()
		}
		return sourceToken, err
	}
	b, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return sourceToken, err
	}

	decodedXML, et, err := getXMLNode(string(b), "GetProfilesResponse")
	if err != nil {
		return sourceToken, err
	}
	var mProfilesResp media.GetProfilesResponse
	if err := decodedXML.DecodeElement(&mProfilesResp, et); err != nil {
		return sourceToken, err
	}

	for i, profile := range mProfilesResp.Profiles {
		if i == profileId && profile.VideoSourceConfiguration != nil && profile.VideoSourceConfiguration.SourceToken != nil {
			return *profile.VideoSourceConfiguration.SourceToken, nil
		}
	}
	return sourceToken, errors.New("no video source token found for profile")
}
