package processor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/jpeg"
	"sync"
	"time"

	"github.com/kerberos-io/agent/examples/frame-processor/contract"
)

const (
	ProfileNeverTrigger        = "never-trigger"
	ProfileAlwaysTrigger       = "always-trigger"
	ProfileEveryNthFrame       = "every-nth-frame"
	ProfileBrightnessThreshold = "brightness-threshold"
)

type Decision struct {
	Triggered bool   `json:"triggered"`
	Reason    string `json:"reason"`
}

type Config struct {
	DefaultProfile      string
	EveryN              int
	BrightnessThreshold uint8
	Delay               time.Duration
	ForceError          bool
}

type Engine struct {
	config Config
	mu     sync.Mutex
	counts map[string]int
}

func IsProfileSupported(profile string) bool {
	switch profile {
	case ProfileNeverTrigger, ProfileAlwaysTrigger, ProfileEveryNthFrame, ProfileBrightnessThreshold:
		return true
	default:
		return false
	}
}

func New(config Config) *Engine {
	if config.DefaultProfile == "" {
		config.DefaultProfile = ProfileNeverTrigger
	}
	if config.EveryN <= 0 {
		config.EveryN = 2
	}
	return &Engine{config: config, counts: make(map[string]int)}
}

func (e *Engine) Process(ctx context.Context, metadata contract.FrameMetadata, frame []byte) (Decision, error) {
	if e.config.Delay > 0 {
		timer := time.NewTimer(e.config.Delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return Decision{}, ctx.Err()
		case <-timer.C:
		}
	}
	if e.config.ForceError {
		return Decision{}, errors.New("configured processing failure")
	}

	profile := metadata.ProcessingProfile
	if profile == "" {
		profile = e.config.DefaultProfile
	}
	switch profile {
	case ProfileNeverTrigger:
		return Decision{Reason: ProfileNeverTrigger}, nil
	case ProfileAlwaysTrigger:
		return Decision{Triggered: true, Reason: ProfileAlwaysTrigger}, nil
	case ProfileEveryNthFrame:
		e.mu.Lock()
		e.counts[metadata.DeviceID]++
		count := e.counts[metadata.DeviceID]
		e.mu.Unlock()
		return Decision{
			Triggered: count%e.config.EveryN == 0,
			Reason:    fmt.Sprintf("frame %d of every %d", count, e.config.EveryN),
		}, nil
	case ProfileBrightnessThreshold:
		image, err := jpeg.Decode(bytes.NewReader(frame))
		if err != nil {
			return Decision{}, fmt.Errorf("decode JPEG: %w", err)
		}
		bounds := image.Bounds()
		var total uint64
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				gray, _, _, _ := image.At(x, y).RGBA()
				total += uint64(gray >> 8)
			}
		}
		pixels := uint64(bounds.Dx() * bounds.Dy())
		if pixels == 0 {
			return Decision{}, errors.New("JPEG has no pixels")
		}
		average := uint8(total / pixels)
		return Decision{
			Triggered: average >= e.config.BrightnessThreshold,
			Reason:    fmt.Sprintf("average brightness %d, threshold %d", average, e.config.BrightnessThreshold),
		}, nil
	default:
		return Decision{}, fmt.Errorf("unknown processing profile %q", profile)
	}
}
