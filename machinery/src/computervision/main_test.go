package computervision

import (
	"testing"

	geo "github.com/kellydunn/golang-geo"
	"github.com/kerberos-io/agent/machinery/src/models"
)

func coordinates(points ...[2]float64) []models.Coordinate {
	var coords []models.Coordinate
	for _, p := range points {
		coords = append(coords, models.Coordinate{X: p[0], Y: p[1]})
	}
	return coords
}

func TestBuildRegionPolygonKeepsConcaveVertices(t *testing.T) {
	tests := []struct {
		name    string
		coords  []models.Coordinate
		inside  [][2]float64
		outside [][2]float64
	}{
		{
			// Two doorways joined along the top: the slot between them must
			// stay outside the region.
			name:    "U shape",
			coords:  coordinates([2]float64{0, 0}, [2]float64{100, 0}, [2]float64{100, 100}, [2]float64{70, 100}, [2]float64{70, 30}, [2]float64{30, 30}, [2]float64{30, 100}, [2]float64{0, 100}),
			inside:  [][2]float64{{15, 80}, {85, 80}, {50, 15}},
			outside: [][2]float64{{50, 50}, {50, 95}, {45, 65}},
		},
		{
			// Square with a V notch cut up from the bottom edge to (60,40).
			name:    "notch",
			coords:  coordinates([2]float64{0, 0}, [2]float64{100, 0}, [2]float64{100, 100}, [2]float64{60, 40}, [2]float64{0, 100}),
			inside:  [][2]float64{{10, 50}, {90, 50}, {50, 20}},
			outside: [][2]float64{{55, 80}, {65, 90}, {50, 95}},
		},
		{
			name:    "L shape",
			coords:  coordinates([2]float64{0, 0}, [2]float64{100, 0}, [2]float64{100, 40}, [2]float64{40, 40}, [2]float64{40, 100}, [2]float64{0, 100}),
			inside:  [][2]float64{{20, 80}, {80, 20}},
			outside: [][2]float64{{70, 70}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			poly := buildRegionPolygon(test.coords, 1, 1)
			if got, want := len(poly.Points()), len(test.coords); got != want {
				t.Fatalf("polygon has %d vertices, want %d", got, want)
			}
			for _, p := range test.inside {
				if !poly.Contains(geo.NewPoint(p[0], p[1])) {
					t.Errorf("point %v should be inside the region", p)
				}
			}
			for _, p := range test.outside {
				if poly.Contains(geo.NewPoint(p[0], p[1])) {
					t.Errorf("point %v should be outside the region", p)
				}
			}
		})
	}
}

func TestBuildRegionPolygonScalesToFrame(t *testing.T) {
	// A region drawn on a 640x360 base, applied to a 1280x720 frame.
	poly := buildRegionPolygon(coordinates([2]float64{0, 0}, [2]float64{320, 0}, [2]float64{320, 180}, [2]float64{0, 180}), 2, 2)
	if !poly.Contains(geo.NewPoint(600, 300)) {
		t.Error("point inside the scaled region should be inside")
	}
	if poly.Contains(geo.NewPoint(700, 300)) {
		t.Error("point outside the scaled region should be outside")
	}
}

func TestBuildRegionPolygonWithTooFewPointsMatchesNothing(t *testing.T) {
	poly := buildRegionPolygon(coordinates([2]float64{0, 0}, [2]float64{100, 100}), 1, 1)
	if poly.Contains(geo.NewPoint(50, 50)) {
		t.Error("a region with fewer than 3 points should not contain any point")
	}
}
