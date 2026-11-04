package onvif

import (
	"encoding/xml"
	"errors"
	"math"
	"strings"
	"testing"
)

type fakeFocusClient struct {
	mode          string
	modeErr       error
	focusModes    []string
	focusModesErr error
	ignoreAuto    bool
	setModeErr    error
	options       *focusMoveOptions
	optionsErr    error
	position      float64
	positionErr   error
	continuousErr error
	relativeErr   error
	absoluteErr   error
	modes         []string
	moves         []focusMove
	stopCalls     int
}

func (client *fakeFocusClient) FocusMode() (string, error) {
	return client.mode, client.modeErr
}

func (client *fakeFocusClient) FocusModes() ([]string, error) {
	if client.focusModes == nil && client.focusModesErr == nil {
		return []string{focusModeManual, focusModeAuto}, nil
	}
	return client.focusModes, client.focusModesErr
}

func (client *fakeFocusClient) SetFocusMode(mode string) error {
	client.modes = append(client.modes, mode)
	if client.setModeErr != nil {
		return client.setModeErr
	}
	if mode == focusModeAuto && client.ignoreAuto {
		return nil
	}
	client.mode = mode
	return nil
}

func (client *fakeFocusClient) MoveOptions() (focusMoveOptions, error) {
	if client.optionsErr != nil {
		return focusMoveOptions{}, client.optionsErr
	}
	if client.options == nil {
		return allFocusMoveOptions, nil
	}
	return *client.options, nil
}

func (client *fakeFocusClient) FocusPosition() (float64, error) {
	return client.position, client.positionErr
}

func (client *fakeFocusClient) MoveFocus(move focusMove) error {
	client.moves = append(client.moves, move)
	switch {
	case move.Continuous != nil:
		return client.continuousErr
	case move.Relative != nil:
		return client.relativeErr
	default:
		return client.absoluteErr
	}
}

func (client *fakeFocusClient) StopFocus() error {
	client.stopCalls++
	return nil
}

func TestContinuousFocusSwitchesToManualMovesAndStops(t *testing.T) {
	client := &fakeFocusClient{mode: focusModeAuto}

	if err := continuousFocus(client, 0.5, 0); err != nil {
		t.Fatalf("continuousFocus() error = %v", err)
	}

	if len(client.modes) != 1 || client.modes[0] != focusModeManual {
		t.Fatalf("modes = %v, want [MANUAL]", client.modes)
	}
	if len(client.moves) != 1 || client.moves[0].Continuous == nil || client.moves[0].Continuous.Speed != 0.5 {
		t.Fatalf("moves = %+v, want one continuous move at 0.5", client.moves)
	}
	if client.stopCalls != 1 {
		t.Fatalf("stopCalls = %d, want 1", client.stopCalls)
	}
}

func TestContinuousFocusKeepsManualMode(t *testing.T) {
	client := &fakeFocusClient{mode: focusModeManual}

	if err := continuousFocus(client, 0.5, 0); err != nil {
		t.Fatalf("continuousFocus() error = %v", err)
	}
	if len(client.modes) != 0 {
		t.Fatalf("modes = %v, want no mode change when already MANUAL", client.modes)
	}
}

func TestContinuousFocusClampsSpeed(t *testing.T) {
	client := &fakeFocusClient{}

	if err := continuousFocus(client, -3, 0); err != nil {
		t.Fatalf("continuousFocus() error = %v", err)
	}
	if got := client.moves[0].Continuous.Speed; got != -1 {
		t.Fatalf("speed = %v, want -1", got)
	}
}

func TestContinuousFocusIgnoresUnsupportedFocusMode(t *testing.T) {
	client := &fakeFocusClient{setModeErr: errors.New("not supported")}

	if err := continuousFocus(client, 0.5, 0); err != nil {
		t.Fatalf("continuousFocus() error = %v", err)
	}
	if len(client.moves) != 1 || client.stopCalls != 1 {
		t.Fatalf("moves = %d stopCalls = %d, want 1 and 1", len(client.moves), client.stopCalls)
	}
}

func TestContinuousFocusFallsBackToRelative(t *testing.T) {
	client := &fakeFocusClient{continuousErr: errors.New("continuous not supported")}

	if err := continuousFocus(client, -0.4, 0); err != nil {
		t.Fatalf("continuousFocus() error = %v", err)
	}
	if len(client.moves) != 2 || client.moves[1].Relative == nil {
		t.Fatalf("moves = %+v, want continuous then relative", client.moves)
	}
	relative := client.moves[1].Relative
	if relative.Distance != -relativeFocusDistance || relative.Speed != 0.4 {
		t.Fatalf("relative = %+v, want distance %v speed 0.4", relative, -relativeFocusDistance)
	}
	if client.stopCalls != 0 {
		t.Fatalf("stopCalls = %d, want 0 for a relative move", client.stopCalls)
	}
}

func TestContinuousFocusUsesAbsoluteWhenOnlyAbsoluteSupported(t *testing.T) {
	// Axis cameras report only absolute focus moves.
	client := &fakeFocusClient{
		mode:     focusModeManual,
		options:  &focusMoveOptions{Absolute: true, AbsoluteMin: 0, AbsoluteMax: 1},
		position: 0.48,
	}

	if err := continuousFocus(client, 0.5, 0); err != nil {
		t.Fatalf("continuousFocus() error = %v", err)
	}
	if len(client.moves) != 1 || client.moves[0].Absolute == nil {
		t.Fatalf("moves = %+v, want a single absolute move", client.moves)
	}
	if got, want := client.moves[0].Absolute.Position, 0.48+0.5*absoluteFocusStep; math.Abs(got-want) > 1e-9 {
		t.Fatalf("position = %v, want %v", got, want)
	}
	if client.stopCalls != 0 {
		t.Fatalf("stopCalls = %d, want 0 for an absolute move", client.stopCalls)
	}
}

func TestContinuousFocusClampsAbsoluteToRange(t *testing.T) {
	client := &fakeFocusClient{
		options:  &focusMoveOptions{Absolute: true, AbsoluteMin: 0, AbsoluteMax: 1},
		position: 0.02,
	}

	if err := continuousFocus(client, -1, 0); err != nil {
		t.Fatalf("continuousFocus() error = %v", err)
	}
	if got := client.moves[0].Absolute.Position; got != 0 {
		t.Fatalf("position = %v, want clamped to 0", got)
	}
}

func TestContinuousFocusTriesAllMovesWithoutOptions(t *testing.T) {
	client := &fakeFocusClient{
		optionsErr:    errors.New("no move options"),
		continuousErr: errors.New("continuous not supported"),
		relativeErr:   errors.New("relative not supported"),
		position:      0.5,
	}

	if err := continuousFocus(client, 0.5, 0); err != nil {
		t.Fatalf("continuousFocus() error = %v", err)
	}
	if len(client.moves) != 3 || client.moves[2].Absolute == nil {
		t.Fatalf("moves = %+v, want continuous, relative then absolute", client.moves)
	}
}

func TestContinuousFocusReturnsErrorWhenAllMovesFail(t *testing.T) {
	client := &fakeFocusClient{
		continuousErr: errors.New("continuous not supported"),
		relativeErr:   errors.New("relative not supported"),
		positionErr:   errors.New("no position"),
	}

	err := continuousFocus(client, 0.5, 0)
	if err == nil || !strings.Contains(err.Error(), "continuous") || !strings.Contains(err.Error(), "relative") || !strings.Contains(err.Error(), "position") {
		t.Fatalf("continuousFocus() error = %v, want every move error", err)
	}
}

func TestContinuousFocusReturnsErrorWhenNoMoveSupported(t *testing.T) {
	client := &fakeFocusClient{options: &focusMoveOptions{}}

	if err := continuousFocus(client, 0.5, 0); err == nil {
		t.Fatal("continuousFocus() error = nil, want an error")
	}
	if len(client.moves) != 0 {
		t.Fatalf("moves = %+v, want none", client.moves)
	}
}

func TestContinuousFocusZeroSpeedStops(t *testing.T) {
	client := &fakeFocusClient{}

	if err := continuousFocus(client, 0, 0); err != nil {
		t.Fatalf("continuousFocus() error = %v", err)
	}
	if len(client.moves) != 0 || len(client.modes) != 0 || client.stopCalls != 1 {
		t.Fatalf("moves = %d modes = %v stopCalls = %d, want only a stop", len(client.moves), client.modes, client.stopCalls)
	}
}

func TestAutoFocusRetriggersWhenAlreadyAuto(t *testing.T) {
	client := &fakeFocusClient{mode: focusModeAuto}

	if err := autoFocus(client, 0); err != nil {
		t.Fatalf("autoFocus() error = %v", err)
	}
	if strings.Join(client.modes, ",") != "MANUAL,AUTO" {
		t.Fatalf("modes = %v, want [MANUAL AUTO]", client.modes)
	}
}

func TestAutoFocusFromManual(t *testing.T) {
	client := &fakeFocusClient{mode: focusModeManual}

	if err := autoFocus(client, 0); err != nil {
		t.Fatalf("autoFocus() error = %v", err)
	}
	if strings.Join(client.modes, ",") != "AUTO" {
		t.Fatalf("modes = %v, want [AUTO]", client.modes)
	}
}

func TestAutoFocusWhenModeUnreadable(t *testing.T) {
	client := &fakeFocusClient{modeErr: errors.New("unreadable")}

	if err := autoFocus(client, 0); err != nil {
		t.Fatalf("autoFocus() error = %v", err)
	}
	if strings.Join(client.modes, ",") != "AUTO" {
		t.Fatalf("modes = %v, want [AUTO]", client.modes)
	}
}

func TestAutoFocusRejectsCameraWithoutAutoMode(t *testing.T) {
	// Dahua cameras advertise only MANUAL.
	client := &fakeFocusClient{mode: focusModeManual, focusModes: []string{focusModeManual}}

	if err := autoFocus(client, 0); !errors.Is(err, errNoNativeAutoFocus) {
		t.Fatalf("autoFocus() error = %v, want errNoNativeAutoFocus", err)
	}
	if len(client.modes) != 0 {
		t.Fatalf("modes = %v, want no mode change", client.modes)
	}
}

func TestAutoFocusDetectsIgnoredAutoMode(t *testing.T) {
	client := &fakeFocusClient{mode: focusModeManual, ignoreAuto: true}

	if err := autoFocus(client, 0); !errors.Is(err, errNoNativeAutoFocus) {
		t.Fatalf("autoFocus() error = %v, want errNoNativeAutoFocus", err)
	}
}

func TestAutoFocusWhenFocusModesUnreadable(t *testing.T) {
	client := &fakeFocusClient{mode: focusModeManual, focusModesErr: errors.New("no options")}

	if err := autoFocus(client, 0); err != nil {
		t.Fatalf("autoFocus() error = %v", err)
	}
	if strings.Join(client.modes, ",") != "AUTO" {
		t.Fatalf("modes = %v, want [AUTO]", client.modes)
	}
}

func TestImagingRequestsOnlyCarryFocusFields(t *testing.T) {
	move, err := xml.Marshal(imagingMoveRequest{
		VideoSourceToken: "source",
		Focus:            focusMove{Continuous: &continuousFocusMove{Speed: 0.5}},
	})
	if err != nil {
		t.Fatalf("marshal move: %v", err)
	}
	wantMove := "<timg:Move><timg:VideoSourceToken>source</timg:VideoSourceToken><timg:Focus><onvif:Continuous><onvif:Speed>0.5</onvif:Speed></onvif:Continuous></timg:Focus></timg:Move>"
	if string(move) != wantMove {
		t.Fatalf("move XML = %s\nwant %s", move, wantMove)
	}

	absolute, err := xml.Marshal(imagingMoveRequest{
		VideoSourceToken: "source",
		Focus:            focusMove{Absolute: &absoluteFocusMove{Position: 0.53}},
	})
	if err != nil {
		t.Fatalf("marshal absolute move: %v", err)
	}
	wantAbsolute := "<timg:Move><timg:VideoSourceToken>source</timg:VideoSourceToken><timg:Focus><onvif:Absolute><onvif:Position>0.53</onvif:Position></onvif:Absolute></timg:Focus></timg:Move>"
	if string(absolute) != wantAbsolute {
		t.Fatalf("absolute move XML = %s\nwant %s", absolute, wantAbsolute)
	}

	request := imagingSetFocusModeRequest{VideoSourceToken: "source"}
	request.ImagingSettings.Focus.AutoFocusMode = focusModeManual
	settings, err := xml.Marshal(request)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	wantSettings := "<timg:SetImagingSettings><timg:VideoSourceToken>source</timg:VideoSourceToken><timg:ImagingSettings><onvif:Focus><onvif:AutoFocusMode>MANUAL</onvif:AutoFocusMode></onvif:Focus></timg:ImagingSettings></timg:SetImagingSettings>"
	if string(settings) != wantSettings {
		t.Fatalf("settings XML = %s\nwant %s", settings, wantSettings)
	}

	// Brightness must come before Focus (schema order) for the Axis retry.
	brightness := 50.0
	request.ImagingSettings.Brightness = &brightness
	settings, err = xml.Marshal(request)
	if err != nil {
		t.Fatalf("marshal settings with brightness: %v", err)
	}
	wantSettings = "<timg:SetImagingSettings><timg:VideoSourceToken>source</timg:VideoSourceToken><timg:ImagingSettings><onvif:Brightness>50</onvif:Brightness><onvif:Focus><onvif:AutoFocusMode>MANUAL</onvif:AutoFocusMode></onvif:Focus></timg:ImagingSettings></timg:SetImagingSettings>"
	if string(settings) != wantSettings {
		t.Fatalf("settings XML = %s\nwant %s", settings, wantSettings)
	}
}

const testImagingEnvelope = `<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://www.w3.org/2003/05/soap-envelope" xmlns:timg="http://www.onvif.org/ver20/imaging/wsdl" xmlns:tt="http://www.onvif.org/ver10/schema"><SOAP-ENV:Body>%s</SOAP-ENV:Body></SOAP-ENV:Envelope>`

func decodeTestImagingResponse(t *testing.T, body string, node string, out interface{}) {
	t.Helper()
	decoder, start, err := getXMLNode(strings.Replace(testImagingEnvelope, "%s", body, 1), node)
	if err != nil {
		t.Fatalf("getXMLNode() error = %v", err)
	}
	if err := decoder.DecodeElement(out, start); err != nil {
		t.Fatalf("DecodeElement() error = %v", err)
	}
}

func TestImagingSettingsResponseDecodesFocusModeAndBrightness(t *testing.T) {
	var settings imagingGetSettingsResponse
	decodeTestImagingResponse(t, `<timg:GetImagingSettingsResponse><timg:ImagingSettings><tt:Brightness>50</tt:Brightness><tt:Focus><tt:AutoFocusMode>AUTO</tt:AutoFocusMode></tt:Focus></timg:ImagingSettings></timg:GetImagingSettingsResponse>`, "GetImagingSettingsResponse", &settings)

	if settings.ImagingSettings.Focus.AutoFocusMode != focusModeAuto {
		t.Fatalf("AutoFocusMode = %q, want AUTO", settings.ImagingSettings.Focus.AutoFocusMode)
	}
	if settings.ImagingSettings.Brightness == nil || *settings.ImagingSettings.Brightness != 50 {
		t.Fatalf("Brightness = %v, want 50", settings.ImagingSettings.Brightness)
	}
}

func TestImagingOptionsResponseDecodesFocusModes(t *testing.T) {
	// Response captured from a Dahua camera.
	var response imagingGetOptionsResponse
	decodeTestImagingResponse(t, `<timg:GetOptionsResponse><timg:ImagingOptions><tt:Brightness><tt:Min>0</tt:Min><tt:Max>100</tt:Max></tt:Brightness><tt:Focus><tt:AutoFocusModes>MANUAL</tt:AutoFocusModes><tt:DefaultSpeed><tt:Min>-1</tt:Min><tt:Max>1</tt:Max></tt:DefaultSpeed></tt:Focus></timg:ImagingOptions></timg:GetOptionsResponse>`, "GetOptionsResponse", &response)

	if got := strings.Join(response.ImagingOptions.Focus.AutoFocusModes, ","); got != "MANUAL" {
		t.Fatalf("AutoFocusModes = %q, want MANUAL", got)
	}
}

func TestMoveOptionsResponseDecodesAbsoluteOnly(t *testing.T) {
	// Response captured from an Axis camera.
	var response imagingGetMoveOptionsResponse
	decodeTestImagingResponse(t, `<timg:GetMoveOptionsResponse><timg:MoveOptions><tt:Absolute><tt:Position><tt:Min>0</tt:Min><tt:Max>1</tt:Max></tt:Position></tt:Absolute></timg:MoveOptions></timg:GetMoveOptionsResponse>`, "GetMoveOptionsResponse", &response)

	options := response.MoveOptions
	if options.Absolute == nil || options.Relative != nil || options.Continuous != nil {
		t.Fatalf("options = %+v, want only absolute", options)
	}
	if options.Absolute.Position.Min != 0 || options.Absolute.Position.Max != 1 {
		t.Fatalf("absolute range = %+v, want 0..1", options.Absolute.Position)
	}
}

func TestStatusResponseDecodesFocusPosition(t *testing.T) {
	// Response captured from an Axis camera.
	var response imagingGetStatusResponse
	decodeTestImagingResponse(t, `<timg:GetStatusResponse><timg:Status><tt:FocusStatus20><tt:Position>0.479226589</tt:Position><tt:MoveStatus>IDLE</tt:MoveStatus></tt:FocusStatus20></timg:Status></timg:GetStatusResponse>`, "GetStatusResponse", &response)

	if response.Status.FocusStatus20 == nil || response.Status.FocusStatus20.Position == nil || *response.Status.FocusStatus20.Position != 0.479226589 {
		t.Fatalf("status = %+v, want position 0.479226589", response.Status)
	}
}

func TestSOAPFaultReasonExtraction(t *testing.T) {
	fault := `<SOAP-ENV:Fault><SOAP-ENV:Reason><SOAP-ENV:Text xml:lang="en">Argument value invalid</SOAP-ENV:Text></SOAP-ENV:Reason></SOAP-ENV:Fault>`
	m := soapFaultReason.FindStringSubmatch(fault)
	if m == nil || m[1] != "Argument value invalid" {
		t.Fatalf("fault reason = %v, want Argument value invalid", m)
	}
}
