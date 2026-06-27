package models

// SelectSubStreamForQuality decides whether the live (HD) view should be served
// from the sub (secondary) RTSP stream for the requested quality tier.
//
// It is resolution-aware: "high" picks whichever configured stream has the
// higher resolution and "low" whichever has the lower resolution, regardless of
// which one is wired as main vs sub. "auto" — the default, also used for the
// empty/unknown value sent by older frontends that never set a quality — keeps
// the historical behaviour of preferring the sub stream when one is available
// (lower bitrate, browser friendly), falling back to the main stream otherwise.
//
// When no sub stream is configured the main stream is always used.
func SelectSubStreamForQuality(config Config, quality string, subStreamEnabled bool) bool {
	if !subStreamEnabled {
		return false
	}

	cam := config.Capture.IPCamera
	mainPixels := cam.Width * cam.Height
	subPixels := cam.SubWidth * cam.SubHeight

	switch quality {
	case StreamQualityHigh:
		// Highest resolution available. If the sub stream is (unusually) larger,
		// use it; otherwise use the main stream. When dimensions are not yet known
		// (0), default to the main stream for "high".
		return subPixels > mainPixels
	case StreamQualityLow:
		// Lowest resolution available. If the main stream is (unusually) the
		// smaller of the two, use it; otherwise use the sub stream. When the sub
		// dimensions are unknown, still prefer the sub stream for "low".
		if mainPixels > 0 && subPixels > 0 && mainPixels < subPixels {
			return false
		}
		return true
	default: // StreamQualityAuto, empty, or any unknown value
		return true
	}
}
