package cloud

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kerberos-io/agent/machinery/src/models"
	agentonvif "github.com/kerberos-io/agent/machinery/src/onvif"
	goonvif "github.com/kerberos-io/onvif"
	goonvifdevice "github.com/kerberos-io/onvif/device"
	goonvifptz "github.com/kerberos-io/onvif/ptz"
)

func TestHeartbeatFailureLogIncludesHubResponse(t *testing.T) {
	response := &http.Response{
		StatusCode: http.StatusBadRequest,
		Status:     "400 Bad Request",
		Body:       io.NopCloser(strings.NewReader(`{"error":"invalid heartbeat"}`)),
	}

	responseBody, truncated, err := readHeartbeatResponseBody(response)
	if err != nil {
		t.Fatalf("readHeartbeatResponseBody() error = %v", err)
	}
	message := formatHeartbeatFailureLog(response, nil, responseBody, truncated, nil, 125*time.Millisecond)

	for _, expected := range []string{
		"status_code=400",
		`status="400 Bad Request"`,
		"duration=125ms",
		`response_body="{\"error\":\"invalid heartbeat\"}"`,
	} {
		if !strings.Contains(message, expected) {
			t.Errorf("formatHeartbeatFailureLog() = %q, want it to contain %q", message, expected)
		}
	}
}

func TestReadHeartbeatResponseBodyTruncatesLargeBody(t *testing.T) {
	response := &http.Response{
		Body: io.NopCloser(strings.NewReader(strings.Repeat("x", heartbeatResponseBodyLogLimit+1))),
	}

	body, truncated, err := readHeartbeatResponseBody(response)
	if err != nil {
		t.Fatalf("readHeartbeatResponseBody() error = %v", err)
	}
	if !truncated {
		t.Fatal("readHeartbeatResponseBody() truncated = false, want true")
	}
	if len(body) != heartbeatResponseBodyLogLimit {
		t.Fatalf("len(body) = %d, want %d", len(body), heartbeatResponseBodyLogLimit)
	}
}

func TestHeartbeatONVIFPayloadCachesStaticAndReusesLoopSubscription(t *testing.T) {
	restoreHeartbeatONVIFStubs(t)

	device := newTestONVIFDevice()
	camera := models.IPCamera{
		ONVIFXAddr:    "http://camera/onvif",
		ONVIFUsername: "operator",
		ONVIFPassword: "secret",
	}
	initialEvents := []agentonvif.ONVIFEvents{{Key: "input-1", Type: "input", Value: "true", Timestamp: 1}}
	loopEvents := []agentonvif.ONVIFEvents{{Key: "output-1", Type: "output", Value: "false", Timestamp: 2}}
	wantPresets := mustJSONMarshal(t, []models.OnvifActionPreset{{Name: "Lobby", Token: "1"}})
	wantInitialEvents := mustJSONMarshal(t, initialEvents)
	wantLoopEvents := mustJSONMarshal(t, loopEvents)

	var connectCalls, ptzConfigCalls, ptzFunctionCalls, presetCalls, createCalls, eventCalls, unsubscribeCalls int

	heartbeatConnectToONVIFDevice = func(*models.IPCamera) (*goonvif.Device, goonvifdevice.GetCapabilitiesResponse, error) {
		connectCalls++
		return device, goonvifdevice.GetCapabilitiesResponse{}, nil
	}
	heartbeatGetPTZConfigurationsFromDevice = func(*goonvif.Device) (goonvifptz.GetConfigurationsResponse, error) {
		ptzConfigCalls++
		return goonvifptz.GetConfigurationsResponse{}, nil
	}
	heartbeatGetPTZFunctionsFromDevice = func(goonvifptz.GetConfigurationsResponse) ([]string, bool, bool) {
		ptzFunctionCalls++
		return nil, true, true
	}
	heartbeatGetPresetsFromDevice = func(*goonvif.Device) ([]models.OnvifActionPreset, error) {
		presetCalls++
		return []models.OnvifActionPreset{{Name: "Lobby", Token: "1"}}, nil
	}
	heartbeatCreatePullPointSubscription = func(*goonvif.Device) (string, error) {
		createCalls++
		switch createCalls {
		case 1:
			return "initial-1", nil
		case 2:
			return "loop", nil
		case 3:
			return "initial-2", nil
		default:
			t.Fatalf("unexpected create pull point call %d", createCalls)
			return "", nil
		}
	}
	heartbeatGetEventMessages = func(_ *goonvif.Device, pullPointAddress string) ([]agentonvif.ONVIFEvents, error) {
		eventCalls++
		switch pullPointAddress {
		case "initial-1", "initial-2":
			return initialEvents, nil
		case "loop":
			return loopEvents, nil
		default:
			t.Fatalf("unexpected pull point address %q", pullPointAddress)
			return nil, nil
		}
	}
	heartbeatUnsubscribePullPoint = func(_ *goonvif.Device, pullPointAddress string) error {
		unsubscribeCalls++
		if pullPointAddress != "initial-1" && pullPointAddress != "initial-2" {
			t.Fatalf("unexpected unsubscribe pull point %q", pullPointAddress)
		}
		return nil
	}

	state := newHeartbeatONVIFState()

	payload := getHeartbeatONVIFPayload(camera, state)
	if payload.enabled != "true" || payload.zoom != "true" || payload.panTilt != "true" || payload.presets != "true" {
		t.Fatalf("unexpected static payload: %+v", payload)
	}
	if string(payload.presetsList) != string(wantPresets) {
		t.Fatalf("payload.presetsList = %s, want %s", payload.presetsList, wantPresets)
	}
	if string(payload.eventsList) != string(wantInitialEvents) {
		t.Fatalf("payload.eventsList = %s, want %s", payload.eventsList, wantInitialEvents)
	}
	if connectCalls != 1 || ptzConfigCalls != 1 || ptzFunctionCalls != 1 || presetCalls != 1 {
		t.Fatalf("unexpected static call counts after first cycle: connect=%d ptzConfig=%d ptzFunctions=%d presets=%d", connectCalls, ptzConfigCalls, ptzFunctionCalls, presetCalls)
	}
	if createCalls != 2 || eventCalls != 1 || unsubscribeCalls != 1 {
		t.Fatalf("unexpected event call counts after first cycle: create=%d events=%d unsubscribe=%d", createCalls, eventCalls, unsubscribeCalls)
	}

	payload = getHeartbeatONVIFPayload(camera, state)
	if string(payload.eventsList) != string(wantLoopEvents) {
		t.Fatalf("second payload.eventsList = %s, want %s", payload.eventsList, wantLoopEvents)
	}
	if connectCalls != 1 {
		t.Fatalf("connectCalls = %d, want 1", connectCalls)
	}
	if ptzConfigCalls != 1 || ptzFunctionCalls != 1 || presetCalls != 1 {
		t.Fatalf("static calls were not cached: ptzConfig=%d ptzFunctions=%d presets=%d", ptzConfigCalls, ptzFunctionCalls, presetCalls)
	}
	if createCalls != 3 || eventCalls != 3 || unsubscribeCalls != 2 {
		t.Fatalf("temporary state subscription was not refreshed or loop subscription was not reused: create=%d events=%d unsubscribe=%d", createCalls, eventCalls, unsubscribeCalls)
	}
}

func TestHeartbeatONVIFPayloadRefreshesAfterCameraConfigChange(t *testing.T) {
	restoreHeartbeatONVIFStubs(t)

	deviceA := newTestONVIFDevice()
	deviceB := newTestONVIFDevice()
	cameraA := models.IPCamera{ONVIFXAddr: "http://camera-a/onvif", ONVIFUsername: "user", ONVIFPassword: "secret-a"}
	cameraB := models.IPCamera{ONVIFXAddr: "http://camera-b/onvif", ONVIFUsername: "user", ONVIFPassword: "secret-b"}
	var connectCalls, ptzConfigCalls, presetCalls, createCalls int
	var unsubscribed []string

	heartbeatConnectToONVIFDevice = func(camera *models.IPCamera) (*goonvif.Device, goonvifdevice.GetCapabilitiesResponse, error) {
		connectCalls++
		switch camera.ONVIFXAddr {
		case cameraA.ONVIFXAddr:
			return deviceA, goonvifdevice.GetCapabilitiesResponse{}, nil
		case cameraB.ONVIFXAddr:
			return deviceB, goonvifdevice.GetCapabilitiesResponse{}, nil
		default:
			t.Fatalf("unexpected camera address %q", camera.ONVIFXAddr)
			return nil, goonvifdevice.GetCapabilitiesResponse{}, nil
		}
	}
	heartbeatGetPTZConfigurationsFromDevice = func(*goonvif.Device) (goonvifptz.GetConfigurationsResponse, error) {
		ptzConfigCalls++
		return goonvifptz.GetConfigurationsResponse{}, nil
	}
	heartbeatGetPTZFunctionsFromDevice = func(goonvifptz.GetConfigurationsResponse) ([]string, bool, bool) {
		return nil, false, true
	}
	heartbeatGetPresetsFromDevice = func(*goonvif.Device) ([]models.OnvifActionPreset, error) {
		presetCalls++
		return nil, nil
	}
	heartbeatCreatePullPointSubscription = func(*goonvif.Device) (string, error) {
		createCalls++
		switch createCalls {
		case 1:
			return "initial-a", nil
		case 2:
			return "loop-a", nil
		case 3:
			return "initial-b", nil
		case 4:
			return "loop-b", nil
		default:
			t.Fatalf("unexpected create pull point call %d", createCalls)
			return "", nil
		}
	}
	heartbeatGetEventMessages = func(_ *goonvif.Device, pullPointAddress string) ([]agentonvif.ONVIFEvents, error) {
		switch pullPointAddress {
		case "initial-a":
			return []agentonvif.ONVIFEvents{{Key: "a", Type: "input", Value: "true", Timestamp: 1}}, nil
		case "initial-b":
			return []agentonvif.ONVIFEvents{{Key: "b", Type: "input", Value: "false", Timestamp: 2}}, nil
		default:
			t.Fatalf("unexpected pull point address %q", pullPointAddress)
			return nil, nil
		}
	}
	heartbeatUnsubscribePullPoint = func(_ *goonvif.Device, pullPointAddress string) error {
		unsubscribed = append(unsubscribed, pullPointAddress)
		return nil
	}

	state := newHeartbeatONVIFState()
	_ = getHeartbeatONVIFPayload(cameraA, state)
	payload := getHeartbeatONVIFPayload(cameraB, state)

	if connectCalls != 2 {
		t.Fatalf("connectCalls = %d, want 2", connectCalls)
	}
	if ptzConfigCalls != 2 || presetCalls != 2 {
		t.Fatalf("camera config change did not refresh static state: ptzConfig=%d presets=%d", ptzConfigCalls, presetCalls)
	}
	if createCalls != 4 {
		t.Fatalf("camera config change did not recreate subscriptions: create=%d", createCalls)
	}
	if countString(unsubscribed, "loop-a") != 1 {
		t.Fatalf("unsubscribed loop-a %d times, want 1; unsubscribed=%v", countString(unsubscribed, "loop-a"), unsubscribed)
	}
	wantEvents := mustJSONMarshal(t, []agentonvif.ONVIFEvents{{Key: "b", Type: "input", Value: "false", Timestamp: 2}})
	if string(payload.eventsList) != string(wantEvents) {
		t.Fatalf("payload.eventsList = %s, want %s", payload.eventsList, wantEvents)
	}
}

func TestHeartbeatONVIFPayloadRetriesStaticFetchFailuresWithoutReconnect(t *testing.T) {
	restoreHeartbeatONVIFStubs(t)

	device := newTestONVIFDevice()
	camera := models.IPCamera{
		ONVIFXAddr:    "http://camera/onvif",
		ONVIFUsername: "operator",
		ONVIFPassword: "secret",
	}
	var connectCalls, ptzConfigCalls, presetCalls, createCalls, eventCalls int

	heartbeatConnectToONVIFDevice = func(*models.IPCamera) (*goonvif.Device, goonvifdevice.GetCapabilitiesResponse, error) {
		connectCalls++
		return device, goonvifdevice.GetCapabilitiesResponse{}, nil
	}
	heartbeatGetPTZConfigurationsFromDevice = func(*goonvif.Device) (goonvifptz.GetConfigurationsResponse, error) {
		ptzConfigCalls++
		return goonvifptz.GetConfigurationsResponse{}, nil
	}
	heartbeatGetPTZFunctionsFromDevice = func(goonvifptz.GetConfigurationsResponse) ([]string, bool, bool) {
		return nil, true, true
	}
	heartbeatGetPresetsFromDevice = func(*goonvif.Device) ([]models.OnvifActionPreset, error) {
		presetCalls++
		if presetCalls == 1 {
			return nil, errors.New("temporary preset failure")
		}
		return []models.OnvifActionPreset{{Name: "Lobby", Token: "1"}}, nil
	}
	heartbeatCreatePullPointSubscription = func(*goonvif.Device) (string, error) {
		createCalls++
		switch createCalls {
		case 1:
			return "initial-1", nil
		case 2:
			return "loop", nil
		case 3:
			return "initial-2", nil
		default:
			t.Fatalf("unexpected create pull point call %d", createCalls)
			return "", nil
		}
	}
	heartbeatGetEventMessages = func(_ *goonvif.Device, pullPointAddress string) ([]agentonvif.ONVIFEvents, error) {
		eventCalls++
		switch pullPointAddress {
		case "initial-1", "initial-2":
			return []agentonvif.ONVIFEvents{{Key: "one", Type: "input", Value: "true", Timestamp: 1}}, nil
		case "loop":
			return []agentonvif.ONVIFEvents{{Key: "two", Type: "output", Value: "false", Timestamp: 2}}, nil
		default:
			t.Fatalf("unexpected pull point address %q", pullPointAddress)
			return nil, nil
		}
	}
	heartbeatUnsubscribePullPoint = func(*goonvif.Device, string) error { return nil }

	state := newHeartbeatONVIFState()
	payload := getHeartbeatONVIFPayload(camera, state)
	if payload.presets != "false" {
		t.Fatalf("payload.presets = %q, want false on transient preset failure", payload.presets)
	}
	payload = getHeartbeatONVIFPayload(camera, state)
	if payload.presets != "true" {
		t.Fatalf("payload.presets = %q, want true after retry", payload.presets)
	}
	if connectCalls != 1 {
		t.Fatalf("connectCalls = %d, want 1", connectCalls)
	}
	if ptzConfigCalls != 2 || presetCalls != 2 {
		t.Fatalf("static failures were not retried on heartbeat cadence: ptzConfig=%d presets=%d", ptzConfigCalls, presetCalls)
	}
	if createCalls != 3 || eventCalls != 3 {
		t.Fatalf("unexpected event behavior during retry: create=%d events=%d", createCalls, eventCalls)
	}
}

func TestHeartbeatONVIFStateReleasesSubscriptionWhenDisabled(t *testing.T) {
	restoreHeartbeatONVIFStubs(t)

	device := newTestONVIFDevice()
	state := newHeartbeatONVIFState()
	state.cameraConfiguration = models.IPCamera{ONVIFXAddr: "http://camera/onvif"}
	state.cameraKey = heartbeatONVIFCameraKey(state.cameraConfiguration)
	state.device = device
	state.loopPullPoint = "loop"

	var unsubscribed []string
	heartbeatUnsubscribePullPoint = func(gotDevice *goonvif.Device, pullPointAddress string) error {
		if gotDevice != device {
			t.Fatal("unsubscribe used a different ONVIF device")
		}
		unsubscribed = append(unsubscribed, pullPointAddress)
		return nil
	}

	state.prepare(models.IPCamera{})

	if len(unsubscribed) != 1 || unsubscribed[0] != "loop" {
		t.Fatalf("unsubscribed = %v, want [loop]", unsubscribed)
	}
	if state.device != nil || state.loopPullPoint != "" {
		t.Fatalf("disabled state retained device or pull point: %+v", state)
	}
}

func TestHeartbeatONVIFPayloadDoesNotCreateSubscriptionsWhenConnectFails(t *testing.T) {
	restoreHeartbeatONVIFStubs(t)

	var createCalls int

	heartbeatConnectToONVIFDevice = func(*models.IPCamera) (*goonvif.Device, goonvifdevice.GetCapabilitiesResponse, error) {
		return nil, goonvifdevice.GetCapabilitiesResponse{}, errors.New("connect failed")
	}
	heartbeatCreatePullPointSubscription = func(*goonvif.Device) (string, error) {
		createCalls++
		return "unexpected", nil
	}

	payload := getHeartbeatONVIFPayload(models.IPCamera{
		ONVIFXAddr:    "http://camera/onvif",
		ONVIFUsername: "operator",
		ONVIFPassword: "secret",
	}, newHeartbeatONVIFState())

	if createCalls != 0 {
		t.Fatalf("createCalls = %d, want 0", createCalls)
	}
	assertDefaultHeartbeatONVIFPayload(t, payload)
}

func TestHeartbeatONVIFPayloadReconnectFailureInvalidatesCache(t *testing.T) {
	restoreHeartbeatONVIFStubs(t)

	device1 := newTestONVIFDevice()
	device2 := newTestONVIFDevice()
	camera := models.IPCamera{
		ONVIFXAddr:    "http://camera/onvif",
		ONVIFUsername: "operator",
		ONVIFPassword: "secret",
	}
	var connectCalls, ptzConfigCalls, presetCalls, createCalls int
	var unsubscribed []string

	heartbeatConnectToONVIFDevice = func(*models.IPCamera) (*goonvif.Device, goonvifdevice.GetCapabilitiesResponse, error) {
		connectCalls++
		switch connectCalls {
		case 1:
			return device1, goonvifdevice.GetCapabilitiesResponse{}, nil
		case 2:
			return device2, goonvifdevice.GetCapabilitiesResponse{}, nil
		default:
			t.Fatalf("unexpected connect call %d", connectCalls)
			return nil, goonvifdevice.GetCapabilitiesResponse{}, nil
		}
	}
	heartbeatGetPTZConfigurationsFromDevice = func(*goonvif.Device) (goonvifptz.GetConfigurationsResponse, error) {
		ptzConfigCalls++
		return goonvifptz.GetConfigurationsResponse{}, nil
	}
	heartbeatGetPTZFunctionsFromDevice = func(goonvifptz.GetConfigurationsResponse) ([]string, bool, bool) {
		return nil, true, false
	}
	heartbeatGetPresetsFromDevice = func(*goonvif.Device) ([]models.OnvifActionPreset, error) {
		presetCalls++
		return []models.OnvifActionPreset{{Name: "Preset", Token: "1"}}, nil
	}
	heartbeatCreatePullPointSubscription = func(*goonvif.Device) (string, error) {
		createCalls++
		switch createCalls {
		case 1:
			return "initial-1", nil
		case 2:
			return "loop-1", nil
		case 3:
			return "initial-2", nil
		case 4:
			return "initial-3", nil
		case 5:
			return "loop-3", nil
		default:
			t.Fatalf("unexpected create pull point call %d", createCalls)
			return "", nil
		}
	}
	heartbeatGetEventMessages = func(_ *goonvif.Device, pullPointAddress string) ([]agentonvif.ONVIFEvents, error) {
		switch pullPointAddress {
		case "initial-1":
			return []agentonvif.ONVIFEvents{{Key: "before", Type: "input", Value: "true", Timestamp: 1}}, nil
		case "initial-2":
			return []agentonvif.ONVIFEvents{{Key: "during", Type: "input", Value: "true", Timestamp: 2}}, nil
		case "loop-1":
			return nil, errors.New("pull failed")
		case "initial-3":
			return []agentonvif.ONVIFEvents{{Key: "after", Type: "input", Value: "false", Timestamp: 2}}, nil
		default:
			t.Fatalf("unexpected pull point address %q", pullPointAddress)
			return nil, nil
		}
	}
	heartbeatUnsubscribePullPoint = func(_ *goonvif.Device, pullPointAddress string) error {
		unsubscribed = append(unsubscribed, pullPointAddress)
		return nil
	}

	state := newHeartbeatONVIFState()
	_ = getHeartbeatONVIFPayload(camera, state)
	payload := getHeartbeatONVIFPayload(camera, state)
	wantEventsDuringFailure := mustJSONMarshal(t, []agentonvif.ONVIFEvents{{Key: "during", Type: "input", Value: "true", Timestamp: 2}})
	if string(payload.eventsList) != string(wantEventsDuringFailure) {
		t.Fatalf("payload.eventsList after operation failure = %s, want %s", payload.eventsList, wantEventsDuringFailure)
	}
	payload = getHeartbeatONVIFPayload(camera, state)

	if ptzConfigCalls != 2 || presetCalls != 2 {
		t.Fatalf("reconnect did not refresh static state: ptzConfig=%d presets=%d", ptzConfigCalls, presetCalls)
	}
	if connectCalls != 2 {
		t.Fatalf("connectCalls = %d, want 2 after reconnect", connectCalls)
	}
	if createCalls != 5 {
		t.Fatalf("reconnect did not recreate subscriptions: create=%d", createCalls)
	}
	if countString(unsubscribed, "loop-1") != 1 {
		t.Fatalf("operation failure cleanup mismatch, unsubscribed=%v", unsubscribed)
	}
	wantEvents := mustJSONMarshal(t, []agentonvif.ONVIFEvents{{Key: "after", Type: "input", Value: "false", Timestamp: 2}})
	if string(payload.eventsList) != string(wantEvents) {
		t.Fatalf("payload.eventsList = %s, want %s", payload.eventsList, wantEvents)
	}
}

func TestHeartbeatONVIFPayloadKeepsCachedConnectionWhenInitialStateFetchFails(t *testing.T) {
	restoreHeartbeatONVIFStubs(t)

	device := newTestONVIFDevice()
	camera := models.IPCamera{
		ONVIFXAddr:    "http://camera/onvif",
		ONVIFUsername: "operator",
		ONVIFPassword: "secret",
	}
	loopEvents := []agentonvif.ONVIFEvents{{Key: "input-1", Type: "input", Value: "true", Timestamp: 1}}

	state := newHeartbeatONVIFState()
	state.cameraConfiguration = camera
	state.cameraKey = heartbeatONVIFCameraKey(camera)
	state.device = device
	state.loopPullPoint = "loop"
	state.staticLoaded = true
	state.staticPayload.enabled = "true"

	heartbeatCreatePullPointSubscription = func(*goonvif.Device) (string, error) {
		return "", errors.New("temporary initial-state failure")
	}
	heartbeatGetEventMessages = func(_ *goonvif.Device, pullPointAddress string) ([]agentonvif.ONVIFEvents, error) {
		if pullPointAddress != "loop" {
			t.Fatalf("unexpected pull point address %q", pullPointAddress)
		}
		return loopEvents, nil
	}

	payload := getHeartbeatONVIFPayload(camera, state)

	wantEvents := mustJSONMarshal(t, loopEvents)
	if string(payload.eventsList) != string(wantEvents) {
		t.Fatalf("payload.eventsList = %s, want %s", payload.eventsList, wantEvents)
	}
	if state.device != device || state.loopPullPoint != "loop" || !state.staticLoaded {
		t.Fatalf("temporary failure invalidated healthy cached state: %+v", state)
	}
}

func restoreHeartbeatONVIFStubs(t *testing.T) {
	t.Helper()

	originalConnect := heartbeatConnectToONVIFDevice
	originalCreate := heartbeatCreatePullPointSubscription
	originalDigitalInputs := heartbeatGetDigitalInputs
	originalEvents := heartbeatGetEventMessages
	originalPresets := heartbeatGetPresetsFromDevice
	originalPTZConfigurations := heartbeatGetPTZConfigurationsFromDevice
	originalPTZFunctions := heartbeatGetPTZFunctionsFromDevice
	originalRelayOutputs := heartbeatGetRelayOutputs
	originalUnsubscribe := heartbeatUnsubscribePullPoint

	t.Cleanup(func() {
		heartbeatConnectToONVIFDevice = originalConnect
		heartbeatCreatePullPointSubscription = originalCreate
		heartbeatGetDigitalInputs = originalDigitalInputs
		heartbeatGetEventMessages = originalEvents
		heartbeatGetPresetsFromDevice = originalPresets
		heartbeatGetPTZConfigurationsFromDevice = originalPTZConfigurations
		heartbeatGetPTZFunctionsFromDevice = originalPTZFunctions
		heartbeatGetRelayOutputs = originalRelayOutputs
		heartbeatUnsubscribePullPoint = originalUnsubscribe
	})
}

func mustJSONMarshal(t *testing.T, v interface{}) []byte {
	t.Helper()

	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return b
}

func assertDefaultHeartbeatONVIFPayload(t *testing.T, payload heartbeatONVIFPayload) {
	t.Helper()

	defaultPayload := defaultHeartbeatONVIFPayload()
	if payload.enabled != defaultPayload.enabled ||
		payload.zoom != defaultPayload.zoom ||
		payload.panTilt != defaultPayload.panTilt ||
		payload.presets != defaultPayload.presets ||
		string(payload.presetsList) != string(defaultPayload.presetsList) ||
		string(payload.eventsList) != string(defaultPayload.eventsList) {
		t.Fatalf("payload = %+v, want default payload", payload)
	}
}

func newTestONVIFDevice() *goonvif.Device {
	return &goonvif.Device{}
}

func countString(values []string, want string) int {
	count := 0
	for _, value := range values {
		if value == want {
			count++
		}
	}
	return count
}
