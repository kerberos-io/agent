package livemoq

import (
	"bytes"
	"testing"

	"github.com/kerberos-io/agent/machinery/src/models"
)

func TestEnsureAnnexB(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		want    []byte
	}{
		{
			name:    "missing start code",
			payload: []byte{0x41, 0x01},
			want:    []byte{0x00, 0x00, 0x00, 0x01, 0x41, 0x01},
		},
		{
			name:    "four byte start code",
			payload: []byte{0x00, 0x00, 0x00, 0x01, 0x65},
			want:    []byte{0x00, 0x00, 0x00, 0x01, 0x65},
		},
		{
			name:    "three byte start code",
			payload: []byte{0x00, 0x00, 0x01, 0x41},
			want:    []byte{0x00, 0x00, 0x01, 0x41},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := EnsureAnnexB(test.payload); !bytes.Equal(got, test.want) {
				t.Fatalf("EnsureAnnexB() = %x, want %x", got, test.want)
			}
		})
	}
}

func TestNormalizeH264AccessUnit(t *testing.T) {
	startCode := []byte{0x00, 0x00, 0x00, 0x01}
	sps := []byte{0x67, 0x42, 0x00, 0x1f}
	pps := []byte{0x68, 0xce, 0x06, 0xe2}
	aud := []byte{0x09, 0xf0}
	idr := []byte{0x65, 0x88, 0x84}

	payload := make([]byte, 0)
	for _, nalu := range [][]byte{sps, pps, aud, sps, pps, idr} {
		payload = append(payload, startCode...)
		payload = append(payload, nalu...)
	}

	got, err := NormalizeH264AccessUnit(payload)
	if err != nil {
		t.Fatal(err)
	}
	want := make([]byte, 0)
	for _, nalu := range [][]byte{sps, pps, idr} {
		want = append(want, startCode...)
		want = append(want, nalu...)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("NormalizeH264AccessUnit() = %x, want %x", got, want)
	}
}

func TestNormalizeH264AccessUnitRemovesOnlyExactDuplicateIDRSlices(t *testing.T) {
	startCode := []byte{0x00, 0x00, 0x00, 0x01}
	idrSlice1 := []byte{0x65, 0x88, 0x84}
	idrSlice2 := []byte{0x65, 0x44, 0x22}

	payload := make([]byte, 0)
	for _, nalu := range [][]byte{idrSlice1, idrSlice1, idrSlice2} {
		payload = append(payload, startCode...)
		payload = append(payload, nalu...)
	}

	got, stats, err := NormalizeH264AccessUnitWithStats(payload)
	if err != nil {
		t.Fatal(err)
	}
	want := make([]byte, 0)
	for _, nalu := range [][]byte{idrSlice1, idrSlice2} {
		want = append(want, startCode...)
		want = append(want, nalu...)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("NormalizeH264AccessUnitWithStats() = %x, want %x", got, want)
	}
	if stats.DuplicateIDRNALUs != 1 {
		t.Fatalf("DuplicateIDRNALUs = %d, want 1", stats.DuplicateIDRNALUs)
	}
}

func TestBroadcastPath(t *testing.T) {
	if got := BroadcastPath("/devices/", "/camera-1/", models.StreamQualityHigh); got != "devices/camera-1/live.hang" {
		t.Fatalf("BroadcastPath() high = %q", got)
	}
	if got := BroadcastPath("", "camera-1", models.StreamQualityHigh); got != "devices/camera-1/live.hang" {
		t.Fatalf("BroadcastPath() default = %q", got)
	}
	if got := BroadcastPath("", "camera-1", models.StreamQualityLow); got != "devices/camera-1/live-low.hang" {
		t.Fatalf("BroadcastPath() low = %q", got)
	}
}

func TestTimestampUs(t *testing.T) {
	if got := TimestampUs(1234); got != 1_234_000 {
		t.Fatalf("TimestampUs() = %d, want 1234000", got)
	}
	if got := TimestampUs(-1); got != 0 {
		t.Fatalf("TimestampUs() negative = %d, want 0", got)
	}
}
