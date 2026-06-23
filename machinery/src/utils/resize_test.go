package utils

import (
	"image"
	"math"
	"testing"
)

func TestResolveBaseDimensions(t *testing.T) {
	tests := []struct {
		name                  string
		baseWidth, baseHeight int
		width, height         int
		wantWidth, wantHeight int
	}{
		{
			name:      "base width set, height derived from aspect ratio",
			baseWidth: 640, baseHeight: 0,
			width: 1920, height: 1080,
			wantWidth: 640, wantHeight: 360,
		},
		{
			name:      "both base dimensions configured are honored",
			baseWidth: 640, baseHeight: 480,
			width: 1920, height: 1080,
			wantWidth: 640, wantHeight: 480,
		},
		{
			name:      "no base configured falls back to source dimensions",
			baseWidth: 0, baseHeight: 0,
			width: 1920, height: 1080,
			wantWidth: 1920, wantHeight: 1080,
		},
		{
			// Regression: a not-yet-probed stream has width=height=0. The old
			// aspect-ratio branch divided by zero (float * +Inf -> MinInt) and
			// poisoned BaseHeight, later crashing resize with makeslice panic.
			name:      "unprobed stream (width=0) does not poison dimensions",
			baseWidth: 640, baseHeight: 0,
			width: 0, height: 0,
			wantWidth: 0, wantHeight: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotWidth, gotHeight := ResolveBaseDimensions(tt.baseWidth, tt.baseHeight, tt.width, tt.height)
			if gotWidth != tt.wantWidth || gotHeight != tt.wantHeight {
				t.Fatalf("ResolveBaseDimensions(%d,%d,%d,%d) = (%d,%d), want (%d,%d)",
					tt.baseWidth, tt.baseHeight, tt.width, tt.height,
					gotWidth, gotHeight, tt.wantWidth, tt.wantHeight)
			}
		})
	}
}

func TestResolveBaseDimensionsNeverNegative(t *testing.T) {
	// Whatever the inputs, the resolved dimensions must never be negative,
	// otherwise the uint cast at the resize call sites wraps to ~MaxUint.
	for _, c := range [][4]int{
		{640, 0, 0, 0},
		{640, 0, 0, 1080},
		{640, 0, 1920, 0},
		{0, 0, 0, 0},
	} {
		w, h := ResolveBaseDimensions(c[0], c[1], c[2], c[3])
		if w < 0 || h < 0 {
			t.Fatalf("ResolveBaseDimensions(%v) produced negative dims (%d,%d)", c, w, h)
		}
	}
}

func TestResizeImageClampsPoisonedDimensions(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 320, 240))

	// uint(math.MinInt) is the value produced when a poisoned int (from
	// int(float * +Inf)) is cast to uint at a call site. It must not panic
	// nfnt/resize's allocator; it should fall back to source-aspect resize.
	// Compute via a runtime int so the conversion doesn't overflow at compile time.
	minInt := math.MinInt
	poison := uint(minInt)

	resized, err := ResizeImage(src, poison, poison)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resized == nil {
		t.Fatalf("expected an image, got nil")
	}
	b := (*resized).Bounds()
	if b.Dx() != 320 || b.Dy() != 240 {
		t.Fatalf("poisoned dims should fall back to source size, got %dx%d", b.Dx(), b.Dy())
	}
}

func TestResizeImageClampsAboveCameraCeiling(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 320, 240))

	// A width beyond any sane camera resolution is treated as "auto" (0).
	resized, err := ResizeImage(src, 100000, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b := (*resized).Bounds()
	if b.Dx() != 320 || b.Dy() != 240 {
		t.Fatalf("oversized width should fall back to source size, got %dx%d", b.Dx(), b.Dy())
	}
}

func TestResizeImageNormalResizeStillWorks(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 1920, 1080))

	resized, err := ResizeImage(src, 640, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b := (*resized).Bounds()
	if b.Dx() != 640 {
		t.Fatalf("expected width 640, got %d", b.Dx())
	}
	if b.Dy() != 360 {
		t.Fatalf("expected aspect-preserved height 360, got %d", b.Dy())
	}
}
