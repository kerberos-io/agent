package livemoq

import (
	"bytes"
	"testing"
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

func TestBroadcastPath(t *testing.T) {
	if got := BroadcastPath("/devices/", "/camera-1/"); got != "devices/camera-1/live.hang" {
		t.Fatalf("BroadcastPath() = %q", got)
	}
	if got := BroadcastPath("", "camera-1"); got != "devices/camera-1/live.hang" {
		t.Fatalf("BroadcastPath() default = %q", got)
	}
}
