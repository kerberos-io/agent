package processor

import (
	"context"
	"testing"

	"github.com/kerberos-io/agent/examples/frame-processor/contract"
)

func TestEveryNthFrameIsTrackedPerDevice(t *testing.T) {
	engine := New(Config{EveryN: 2})
	metadata := contract.FrameMetadata{ProcessingProfile: ProfileEveryNthFrame, DeviceID: "device-1"}

	first, err := engine.Process(context.Background(), metadata, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := engine.Process(context.Background(), metadata, nil)
	if err != nil {
		t.Fatal(err)
	}
	metadata.DeviceID = "device-2"
	otherDevice, err := engine.Process(context.Background(), metadata, nil)
	if err != nil {
		t.Fatal(err)
	}

	if first.Triggered || !second.Triggered || otherDevice.Triggered {
		t.Fatalf("decisions = first:%t second:%t other:%t", first.Triggered, second.Triggered, otherDevice.Triggered)
	}
}

func TestUnknownProfileFails(t *testing.T) {
	engine := New(Config{})
	_, err := engine.Process(context.Background(), contract.FrameMetadata{
		ProcessingProfile: "missing-profile",
	}, nil)
	if err == nil {
		t.Fatal("Process() accepted an unknown profile")
	}
}
