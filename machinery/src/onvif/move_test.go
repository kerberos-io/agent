package onvif

import (
	"encoding/xml"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kerberos-io/onvif/ptz"
	xsdonvif "github.com/kerberos-io/onvif/xsd/onvif"
)

type fakePTZMoveClient struct {
	current              ptzTarget
	target               ptzTarget
	lastVelocity         ptzVelocity
	absoluteErr          error
	continuousErr        error
	stopErr              error
	ignoreAbsolute       bool
	stalled              bool
	incompletePosition   bool
	positionErr          error
	positionErrAtCall    int
	positionCalls        int
	absoluteCalls        int
	continuousVelocities []ptzVelocity
	stopCalls            int
	moving               bool
}

func (client *fakePTZMoveClient) GetPosition() (xsdonvif.PTZVector, error) {
	client.positionCalls++
	if client.positionErr != nil && client.positionCalls == client.positionErrAtCall {
		return xsdonvif.PTZVector{}, client.positionErr
	}
	if client.moving && !client.stalled {
		client.current.Pan = moveFakeAxis(client.current.Pan, client.target.Pan, client.lastVelocity.Pan)
		client.current.Tilt = moveFakeAxis(client.current.Tilt, client.target.Tilt, client.lastVelocity.Tilt)
		client.current.Zoom = moveFakeAxis(client.current.Zoom, client.target.Zoom, client.lastVelocity.Zoom)
	}

	panTilt := &xsdonvif.Vector2D{X: client.current.Pan, Y: client.current.Tilt}
	if client.incompletePosition {
		return xsdonvif.PTZVector{PanTilt: panTilt}, nil
	}
	return xsdonvif.PTZVector{
		PanTilt: panTilt,
		Zoom:    &xsdonvif.Vector1D{X: client.current.Zoom},
	}, nil
}

func (client *fakePTZMoveClient) AbsoluteMove(target ptzTarget) error {
	client.absoluteCalls++
	client.target = target
	if client.absoluteErr != nil {
		return client.absoluteErr
	}
	if !client.ignoreAbsolute {
		client.current = target
	}
	return nil
}

func (client *fakePTZMoveClient) ContinuousMove(velocity ptzVelocity) error {
	if client.continuousErr != nil {
		return client.continuousErr
	}
	client.lastVelocity = velocity
	client.continuousVelocities = append(client.continuousVelocities, velocity)
	client.moving = true
	return nil
}

func (client *fakePTZMoveClient) Stop() error {
	client.stopCalls++
	client.moving = false
	return client.stopErr
}

func moveFakeAxis(current float64, target float64, velocity float64) float64 {
	step := velocity * 0.5
	if step == 0 {
		return current
	}
	next := current + step
	if (velocity > 0 && next >= target) || (velocity < 0 && next <= target) {
		return target
	}
	return next
}

func testPTZMoveOptions() ptzMoveOptions {
	options := defaultPTZMoveOptions
	options.NativeTimeout = 50 * time.Millisecond
	options.ContinuousTimeout = 100 * time.Millisecond
	options.NoProgressTimeout = 10 * time.Millisecond
	options.PollInterval = time.Millisecond
	options.ProgressEpsilon = 0.0001
	options.VelocityChange = 0.01
	return options
}

func TestMoveToPositionUsesVerifiedNativeMove(t *testing.T) {
	target := ptzTarget{Pan: 0.5, Tilt: -0.25, Zoom: 0.4}
	client := &fakePTZMoveClient{}

	err := moveToPosition(client, ptzMoveSupport{Absolute: true, Continuous: true}, target, testPTZMoveOptions())
	if err != nil {
		t.Fatalf("moveToPosition() error = %v", err)
	}
	if client.absoluteCalls != 1 {
		t.Fatalf("absolute calls = %d, want 1", client.absoluteCalls)
	}
	if len(client.continuousVelocities) != 0 {
		t.Fatalf("continuous calls = %d, want 0", len(client.continuousVelocities))
	}
}

func TestMoveToPositionFallsBackToSimultaneousContinuousMove(t *testing.T) {
	target := ptzTarget{Pan: 0.5, Tilt: -0.25, Zoom: 0.4}
	client := &fakePTZMoveClient{
		target:      target,
		absoluteErr: errors.New("absolute move rejected"),
	}

	err := moveToPosition(client, ptzMoveSupport{Absolute: true, Continuous: true}, target, testPTZMoveOptions())
	if err != nil {
		t.Fatalf("moveToPosition() error = %v", err)
	}
	if len(client.continuousVelocities) == 0 {
		t.Fatal("continuous fallback was not used")
	}
	first := client.continuousVelocities[0]
	if first.Pan == 0 || first.Tilt == 0 || first.Zoom == 0 {
		t.Fatalf("first fallback velocity = %+v, want all axes moving", first)
	}
	if client.stopCalls != 2 {
		t.Fatalf("stop calls = %d, want native cleanup and fallback cleanup", client.stopCalls)
	}
}

func TestMoveToPositionFallsBackWhenNativeMoveMakesNoProgress(t *testing.T) {
	target := ptzTarget{Pan: 0.3, Tilt: 0.2, Zoom: 0.1}
	client := &fakePTZMoveClient{
		target:         target,
		ignoreAbsolute: true,
	}
	options := testPTZMoveOptions()
	options.NoProgressTimeout = 3 * time.Millisecond

	err := moveToPosition(client, ptzMoveSupport{Absolute: true, Continuous: true}, target, options)
	if err != nil {
		t.Fatalf("moveToPosition() error = %v", err)
	}
	if len(client.continuousVelocities) == 0 {
		t.Fatal("continuous fallback was not used")
	}
}

func TestMoveToPositionStopsAfterContinuousStatusError(t *testing.T) {
	target := ptzTarget{Pan: 0.3, Tilt: 0.2, Zoom: 0.1}
	client := &fakePTZMoveClient{
		target:            target,
		positionErr:       errors.New("status unavailable"),
		positionErrAtCall: 3,
	}

	err := moveToPosition(client, ptzMoveSupport{Continuous: true}, target, testPTZMoveOptions())
	if err == nil || !strings.Contains(err.Error(), "status unavailable") {
		t.Fatalf("moveToPosition() error = %v, want status error", err)
	}
	if client.stopCalls != 1 {
		t.Fatalf("stop calls = %d, want 1", client.stopCalls)
	}
}

func TestMoveToPositionStopsWhenContinuousMoveIsRejected(t *testing.T) {
	target := ptzTarget{Pan: 0.3, Tilt: 0.2, Zoom: 0.1}
	client := &fakePTZMoveClient{
		target:        target,
		continuousErr: errors.New("continuous move rejected"),
	}

	err := moveToPosition(client, ptzMoveSupport{Continuous: true}, target, testPTZMoveOptions())
	if err == nil || !strings.Contains(err.Error(), "continuous move rejected") {
		t.Fatalf("moveToPosition() error = %v, want movement error", err)
	}
	if client.stopCalls != 1 {
		t.Fatalf("stop calls = %d, want 1", client.stopCalls)
	}
}

func TestMoveToPositionReturnsNoProgressErrorAndStops(t *testing.T) {
	target := ptzTarget{Pan: 0.3, Tilt: 0.2, Zoom: 0.1}
	client := &fakePTZMoveClient{
		target:  target,
		stalled: true,
	}
	options := testPTZMoveOptions()
	options.NoProgressTimeout = 3 * time.Millisecond

	err := moveToPosition(client, ptzMoveSupport{Continuous: true}, target, options)
	if err == nil || !strings.Contains(err.Error(), "made no progress") {
		t.Fatalf("moveToPosition() error = %v, want no-progress error", err)
	}
	if client.stopCalls != 1 {
		t.Fatalf("stop calls = %d, want 1", client.stopCalls)
	}
}

func TestMoveToPositionReturnsDeadlineErrorAndStops(t *testing.T) {
	target := ptzTarget{Pan: 100, Tilt: 100, Zoom: 100}
	client := &fakePTZMoveClient{target: target}
	options := testPTZMoveOptions()
	options.ContinuousTimeout = 3 * time.Millisecond
	options.NoProgressTimeout = 50 * time.Millisecond

	err := moveToPosition(client, ptzMoveSupport{Continuous: true}, target, options)
	if err == nil || !strings.Contains(err.Error(), "target not reached within") {
		t.Fatalf("moveToPosition() error = %v, want deadline error", err)
	}
	if client.stopCalls != 1 {
		t.Fatalf("stop calls = %d, want 1", client.stopCalls)
	}
}

func TestMoveToPositionRejectsIncompletePosition(t *testing.T) {
	client := &fakePTZMoveClient{incompletePosition: true}

	err := moveToPosition(client, ptzMoveSupport{Continuous: true}, ptzTarget{Pan: 0.1}, testPTZMoveOptions())
	if err == nil || !strings.Contains(err.Error(), "incomplete PTZ position") {
		t.Fatalf("moveToPosition() error = %v, want incomplete-position error", err)
	}
}

func TestCombinedContinuousMoveVelocityIncludesEveryAxis(t *testing.T) {
	request := ptz.ContinuousMove{
		Velocity: ptz.Speed{
			PanTilt: &xsdonvif.Vector2D{X: 0.5, Y: -0.25},
			Zoom:    &xsdonvif.Vector1D{X: 0.2},
		},
	}

	encoded, err := xml.Marshal(request)
	if err != nil {
		t.Fatalf("xml.Marshal() error = %v", err)
	}
	xmlBody := string(encoded)
	if !strings.Contains(xmlBody, `PanTilt x="0.5" y="-0.25"`) {
		t.Fatalf("ContinuousMove XML = %s, want pan and tilt", xmlBody)
	}
	if !strings.Contains(xmlBody, `Zoom x="0.2"`) {
		t.Fatalf("ContinuousMove XML = %s, want zoom", xmlBody)
	}
}

func TestGetPTZFunctionsHandlesMissingConfiguration(t *testing.T) {
	functions, canZoom, canPanTilt := GetPTZFunctionsFromDevice(ptz.GetConfigurationsResponse{})
	if len(functions) != 0 || canZoom || canPanTilt {
		t.Fatalf("GetPTZFunctionsFromDevice() = %v, %t, %t; want empty capabilities", functions, canZoom, canPanTilt)
	}
}
