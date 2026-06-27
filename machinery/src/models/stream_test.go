package models

import "testing"

func cfgWithDims(mainW, mainH, subW, subH int) Config {
	c := Config{}
	c.Capture.IPCamera.Width = mainW
	c.Capture.IPCamera.Height = mainH
	c.Capture.IPCamera.SubWidth = subW
	c.Capture.IPCamera.SubHeight = subH
	return c
}

func TestSelectSubStreamForQuality(t *testing.T) {
	tests := []struct {
		name             string
		config           Config
		quality          string
		subStreamEnabled bool
		wantSub          bool
	}{
		// No sub stream configured -> always the main stream.
		{"no sub, auto", cfgWithDims(1920, 1080, 0, 0), StreamQualityAuto, false, false},
		{"no sub, high", cfgWithDims(1920, 1080, 0, 0), StreamQualityHigh, false, false},
		{"no sub, low", cfgWithDims(1920, 1080, 0, 0), StreamQualityLow, false, false},

		// Typical config: main is the bigger stream, sub the smaller one.
		{"auto prefers sub", cfgWithDims(1920, 1080, 640, 480), StreamQualityAuto, true, true},
		{"empty prefers sub", cfgWithDims(1920, 1080, 640, 480), "", true, true},
		{"unknown prefers sub", cfgWithDims(1920, 1080, 640, 480), "potato", true, true},
		{"high picks main", cfgWithDims(1920, 1080, 640, 480), StreamQualityHigh, true, false},
		{"low picks sub", cfgWithDims(1920, 1080, 640, 480), StreamQualityLow, true, true},

		// Dimensions not probed yet (0): high defaults to main, low/auto to sub.
		{"unknown dims, high", cfgWithDims(0, 0, 0, 0), StreamQualityHigh, true, false},
		{"unknown dims, low", cfgWithDims(0, 0, 0, 0), StreamQualityLow, true, true},
		{"unknown dims, auto", cfgWithDims(0, 0, 0, 0), StreamQualityAuto, true, true},

		// Inverted config: sub is (unusually) the higher-resolution stream.
		{"inverted high picks sub", cfgWithDims(640, 480, 1920, 1080), StreamQualityHigh, true, true},
		{"inverted low picks main", cfgWithDims(640, 480, 1920, 1080), StreamQualityLow, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SelectSubStreamForQuality(tt.config, tt.quality, tt.subStreamEnabled)
			if got != tt.wantSub {
				t.Errorf("SelectSubStreamForQuality(quality=%q, subEnabled=%v) = %v, want %v",
					tt.quality, tt.subStreamEnabled, got, tt.wantSub)
			}
		})
	}
}
