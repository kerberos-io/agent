package capture

import (
	"bytes"
	"context"
	"encoding/pem"
	"errors"
	"math"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/headers"
	"github.com/bluenviron/mediacommon/pkg/codecs/h264"
)

func TestPreRecordingGOPCount(t *testing.T) {
	maxInt := int64(^uint(0) >> 1)
	tests := []struct {
		name         string
		preRecording int64
		gopDuration  float64
		want         int
		wantOK       bool
	}{
		{name: "normal duration", preRecording: 10, gopDuration: 2.9, want: 6, wantOK: true},
		{name: "duration longer than buffer", preRecording: 1, gopDuration: 2, want: 1, wantOK: true},
		{name: "largest representable result", preRecording: maxInt - 1, gopDuration: 1, want: int(maxInt), wantOK: true},
		{name: "result exceeds int", preRecording: maxInt, gopDuration: 1, wantOK: false},
		{name: "non-positive pre-recording", preRecording: 0, gopDuration: 1, wantOK: false},
		{name: "sub-second GOP", preRecording: 10, gopDuration: 0.9, wantOK: false},
		{name: "NaN GOP", preRecording: 10, gopDuration: math.NaN(), wantOK: false},
		{name: "infinite GOP", preRecording: 10, gopDuration: math.Inf(1), wantOK: false},
		{name: "GOP exceeds int64", preRecording: 10, gopDuration: float64(math.MaxInt64), wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := preRecordingGOPCount(tt.preRecording, tt.gopDuration)
			if ok != tt.wantOK || got != tt.want {
				t.Fatalf("preRecordingGOPCount(%d, %v) = (%d, %t), want (%d, %t)",
					tt.preRecording, tt.gopDuration, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestGolibrtspCloseBeforeClientStart(t *testing.T) {
	client := &Golibrtsp{}

	if err := client.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestSanitizeRTSPErrorRemovesCredentialsAndQuery(t *testing.T) {
	rawURL := "rtsp://camera-user:camera-password@10.0.20.15/live?access_token=secret"
	got := sanitizeRTSPError(errors.New("describe "+rawURL+": bad status code"), rawURL)

	for _, secret := range []string{"camera-user", "camera-password", "access_token", "secret"} {
		if strings.Contains(got.Error(), secret) {
			t.Fatalf("sanitizeRTSPError() exposed %q in %q", secret, got)
		}
	}
	if !strings.Contains(got.Error(), "rtsp://10.0.20.15/live") {
		t.Fatalf("sanitizeRTSPError() removed useful host/path context: %q", got)
	}
}

func TestEscapeRTSPUserinfo(t *testing.T) {
	tests := []struct {
		name         string
		rawURL       string
		want         string
		wantUsername string
		wantPassword string
		wantHost     string
	}{
		{
			name:         "caret in password",
			rawURL:       "rtsp://admin:pass^word@10.0.0.1:554/stream1",
			want:         "rtsp://admin:pass%5Eword@10.0.0.1:554/stream1",
			wantUsername: "admin",
			wantPassword: "pass^word",
			wantHost:     "10.0.0.1:554",
		},
		{
			name:         "space, quotes and braces",
			rawURL:       `rtsps://cam user:my "p{a}ss"@camera.local:322/live`,
			want:         "rtsps://cam%20user:my%20%22p%7Ba%7Dss%22@camera.local:322/live",
			wantUsername: "cam user",
			wantPassword: `my "p{a}ss"`,
			wantHost:     "camera.local:322",
		},
		{
			name:         "at sign next to an escaped character",
			rawURL:       "rtsp://admin:p@ss^@10.0.0.1/stream",
			want:         "rtsp://admin:p%40ss%5E@10.0.0.1/stream",
			wantUsername: "admin",
			wantPassword: "p@ss^",
			wantHost:     "10.0.0.1",
		},
		{
			name:         "existing escapes are kept",
			rawURL:       "rtsp://admin:pass%5Eword@10.0.0.1/stream",
			want:         "rtsp://admin:pass%5Eword@10.0.0.1/stream",
			wantUsername: "admin",
			wantPassword: "pass^word",
			wantHost:     "10.0.0.1",
		},
		{
			name:         "stray percent sign",
			rawURL:       "rtsp://admin:100%@10.0.0.1/stream",
			want:         "rtsp://admin:100%25@10.0.0.1/stream",
			wantUsername: "admin",
			wantPassword: "100%",
			wantHost:     "10.0.0.1",
		},
		{
			name:         "non-ASCII password",
			rawURL:       "rtsp://admin:pässwörd@10.0.0.1/stream",
			want:         "rtsp://admin:p%C3%A4ssw%C3%B6rd@10.0.0.1/stream",
			wantUsername: "admin",
			wantPassword: "pässwörd",
			wantHost:     "10.0.0.1",
		},
		{
			name:         "IPv6 host",
			rawURL:       "rtsp://admin:pa^ss@[fe80::1]:554/stream",
			want:         "rtsp://admin:pa%5Ess@[fe80::1]:554/stream",
			wantUsername: "admin",
			wantPassword: "pa^ss",
			wantHost:     "[fe80::1]:554",
		},
		{
			name:         "valid characters are unchanged",
			rawURL:       "rtsp://admin:a-b._~!$&'()*+,;=:c@10.0.0.1/stream?channel=1&subtype=0",
			want:         "rtsp://admin:a-b._~!$&'()*+,;=:c@10.0.0.1/stream?channel=1&subtype=0",
			wantUsername: "admin",
			wantPassword: "a-b._~!$&'()*+,;=:c",
			wantHost:     "10.0.0.1",
		},
		{
			name:     "no credentials",
			rawURL:   "rtsp://10.0.0.1:554/stream?owner=user@example.com",
			want:     "rtsp://10.0.0.1:554/stream?owner=user@example.com",
			wantHost: "10.0.0.1:554",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := escapeRTSPUserinfo(test.rawURL)
			if got != test.want {
				t.Fatalf("escapeRTSPUserinfo(%q) = %q, want %q", test.rawURL, got, test.want)
			}

			u, err := base.ParseURL(got)
			if err != nil {
				t.Fatalf("base.ParseURL(%q) error = %v", got, err)
			}
			var username, password string
			if u.User != nil {
				username = u.User.Username()
				password, _ = u.User.Password()
			}
			if username != test.wantUsername || password != test.wantPassword {
				t.Fatalf("credentials = (%q, %q), want (%q, %q)", username, password, test.wantUsername, test.wantPassword)
			}
			if u.Host != test.wantHost {
				t.Fatalf("host = %q, want %q", u.Host, test.wantHost)
			}
		})
	}
}

func TestConnectAcceptsSpecialCharactersInPassword(t *testing.T) {
	// Reserve a local port and close it again, so connecting is refused.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	address := listener.Addr().String()
	listener.Close()

	client := &Golibrtsp{Url: "rtsp://admin:pass^word@" + address + "/stream"}
	err = client.Connect(context.Background(), context.Background())
	t.Cleanup(func() { client.Close(context.Background()) })

	if err == nil {
		t.Fatal("Connect() error = nil, want a connection error")
	}
	if strings.Contains(err.Error(), "invalid userinfo") {
		t.Fatalf("Connect() could not parse the camera URL: %v", err)
	}
	if strings.Contains(err.Error(), "pass") {
		t.Fatalf("Connect() exposed the camera password in %q", err)
	}
}

func TestRTSPSTLSConfig(t *testing.T) {
	t.Run("verifies certificates by default", func(t *testing.T) {
		t.Setenv(rtspsCAFileEnv, "")
		t.Setenv(rtspsInsecureEnv, "")

		got, err := rtspsTLSConfig()
		if err != nil {
			t.Fatalf("rtspsTLSConfig() error = %v", err)
		}
		if got != nil {
			t.Fatalf("rtspsTLSConfig() = %#v, want nil", got)
		}
	})

	t.Run("allows explicit insecure mode", func(t *testing.T) {
		t.Setenv(rtspsCAFileEnv, "/missing/ignored-in-insecure-mode.pem")
		t.Setenv(rtspsInsecureEnv, "true")

		got, err := rtspsTLSConfig()
		if err != nil {
			t.Fatalf("rtspsTLSConfig() error = %v", err)
		}
		if got == nil || !got.InsecureSkipVerify {
			t.Fatalf("rtspsTLSConfig() = %#v, want InsecureSkipVerify enabled", got)
		}
	})

	t.Run("adds a camera CA to system roots", func(t *testing.T) {
		t.Setenv(rtspsInsecureEnv, "")
		server := httptest.NewTLSServer(nil)
		defer server.Close()

		certificate := server.Certificate()
		caFile := filepath.Join(t.TempDir(), "camera-ca.pem")
		caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
		if err := os.WriteFile(caFile, caPEM, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv(rtspsCAFileEnv, caFile)

		got, err := rtspsTLSConfig()
		if err != nil {
			t.Fatalf("rtspsTLSConfig() error = %v", err)
		}
		if got == nil || got.RootCAs == nil {
			t.Fatalf("rtspsTLSConfig() = %#v, want custom RootCAs", got)
		}
		for _, subject := range got.RootCAs.Subjects() {
			if bytes.Equal(subject, certificate.RawSubject) {
				return
			}
		}
		t.Fatal("camera CA was not added to RootCAs")
	})

	t.Run("rejects an invalid camera CA file", func(t *testing.T) {
		t.Setenv(rtspsInsecureEnv, "")
		caFile := filepath.Join(t.TempDir(), "camera-ca.pem")
		if err := os.WriteFile(caFile, []byte("not a certificate"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv(rtspsCAFileEnv, caFile)

		if _, err := rtspsTLSConfig(); err == nil {
			t.Fatal("rtspsTLSConfig() error = nil, want invalid CA error")
		}
	})
}

func TestVideoReferenceStateDropsDependentFramesUntilRandomAccess(t *testing.T) {
	state := &videoReferenceState{}

	accepted, recovered := state.accept(false)
	if accepted || recovered != 0 || state.droppedAccessUnits != 1 {
		t.Fatalf("initial dependent frame = (accepted=%t, recovered=%d, dropped=%d), want (false, 0, 1)",
			accepted, recovered, state.droppedAccessUnits)
	}

	accepted, recovered = state.accept(true)
	if !accepted || recovered != 1 || !state.valid {
		t.Fatalf("random-access recovery = (accepted=%t, recovered=%d, valid=%t), want (true, 1, true)",
			accepted, recovered, state.valid)
	}

	if expected, gap := state.observeSequence(100); gap || expected != 100 {
		t.Fatalf("first sequence observation = (expected=%d, gap=%t), want (100, false)", expected, gap)
	}
	if expected, gap := state.observeSequence(102); !gap || expected != 101 || state.valid {
		t.Fatalf("sequence gap = (expected=%d, gap=%t, valid=%t), want (101, true, false)",
			expected, gap, state.valid)
	}

	accepted, recovered = state.accept(false)
	if accepted || recovered != 0 || state.droppedAccessUnits != 1 {
		t.Fatalf("post-loss dependent frame = (accepted=%t, recovered=%d, dropped=%d), want (false, 0, 1)",
			accepted, recovered, state.droppedAccessUnits)
	}

	accepted, recovered = state.accept(true)
	if !accepted || recovered != 1 || !state.valid {
		t.Fatalf("post-loss recovery = (accepted=%t, recovered=%d, valid=%t), want (true, 1, true)",
			accepted, recovered, state.valid)
	}
}

func TestVideoReferenceStateAcceptsSequenceWraparound(t *testing.T) {
	state := &videoReferenceState{valid: true}

	if _, gap := state.observeSequence(^uint16(0)); gap {
		t.Fatal("first sequence observation reported a gap")
	}
	if expected, gap := state.observeSequence(0); gap || expected != 0 || !state.valid {
		t.Fatalf("sequence wraparound = (expected=%d, gap=%t, valid=%t), want (0, false, true)",
			expected, gap, state.valid)
	}
}

func TestH264FrameNumTrackerDetectsMissingIDR(t *testing.T) {
	tracker := &h264FrameNumTracker{}
	tracker.setSPS(&h264.SPS{
		Log2MaxFrameNumMinus4: 1,
		FrameMbsOnlyFlag:      true,
	})

	if _, _, gap, ok := tracker.observe([][]byte{testH264Slice(h264.NALUTypeIDR, 0)}); !ok || gap {
		t.Fatalf("initial IDR observation = (ok=%t, gap=%t), want (true, false)", ok, gap)
	}
	for frameNum := uint32(1); frameNum <= 24; frameNum++ {
		if expected, received, gap, ok := tracker.observe([][]byte{testH264Slice(h264.NALUTypeNonIDR, frameNum)}); !ok || gap {
			t.Fatalf("frame_num %d observation = (expected=%d, received=%d, ok=%t, gap=%t), want no gap",
				frameNum, expected, received, ok, gap)
		}
	}

	expected, received, gap, ok := tracker.observe([][]byte{testH264Slice(h264.NALUTypeNonIDR, 1)})
	if !ok || !gap || expected != 25 || received != 1 {
		t.Fatalf("missing IDR transition = (expected=%d, received=%d, ok=%t, gap=%t), want (25, 1, true, true)",
			expected, received, ok, gap)
	}
}

func TestH264FrameNumTrackerResetsAtIDR(t *testing.T) {
	tracker := &h264FrameNumTracker{}
	tracker.setSPS(&h264.SPS{
		Log2MaxFrameNumMinus4: 1,
		FrameMbsOnlyFlag:      true,
	})

	tracker.observe([][]byte{testH264Slice(h264.NALUTypeIDR, 0)})
	tracker.observe([][]byte{testH264Slice(h264.NALUTypeNonIDR, 1)})
	if expected, received, gap, ok := tracker.observe([][]byte{testH264Slice(h264.NALUTypeIDR, 0)}); !ok || gap {
		t.Fatalf("IDR reset = (expected=%d, received=%d, ok=%t, gap=%t), want no gap",
			expected, received, ok, gap)
	}
	if expected, received, gap, ok := tracker.observe([][]byte{testH264Slice(h264.NALUTypeNonIDR, 1)}); !ok || gap || expected != 1 || received != 1 {
		t.Fatalf("post-IDR frame = (expected=%d, received=%d, ok=%t, gap=%t), want (1, 1, true, false)",
			expected, received, ok, gap)
	}
}

func TestH264FrameNumTrackerHonorsPermittedGaps(t *testing.T) {
	tracker := &h264FrameNumTracker{}
	tracker.setSPS(&h264.SPS{
		Log2MaxFrameNumMinus4:          1,
		FrameMbsOnlyFlag:               true,
		GapsInFrameNumValueAllowedFlag: true,
	})

	tracker.observe([][]byte{testH264Slice(h264.NALUTypeIDR, 0)})
	expected, received, gap, ok := tracker.observe([][]byte{testH264Slice(h264.NALUTypeNonIDR, 2)})
	if !ok || gap || expected != 1 || received != 2 {
		t.Fatalf("permitted frame_num gap = (expected=%d, received=%d, ok=%t, gap=%t), want (1, 2, true, false)",
			expected, received, ok, gap)
	}
}

func testH264Slice(naluType h264.NALUType, frameNum uint32) []byte {
	return []byte{0x60 | byte(naluType), 0xE0 | byte(frameNum&0x1F)}
}

func TestRTSPProtocol(t *testing.T) {
	tcp := gortsplib.ProtocolTCP
	udp := gortsplib.ProtocolUDP
	tests := []struct {
		transport string
		want      *gortsplib.Protocol
	}{
		{transport: "", want: &tcp},
		{transport: "tcp", want: &tcp},
		{transport: " TCP ", want: &tcp},
		{transport: "udp", want: &udp},
		{transport: "UDP", want: &udp},
		{transport: "auto", want: nil},
		{transport: "Auto", want: nil},
		{transport: "bogus", want: &tcp},
	}
	for _, test := range tests {
		t.Run(test.transport, func(t *testing.T) {
			got := rtspProtocol(test.transport)
			if test.want == nil {
				if got != nil {
					t.Fatalf("rtspProtocol(%q) = %v, want nil (auto)", test.transport, *got)
				}
				return
			}
			if got == nil || *got != *test.want {
				t.Fatalf("rtspProtocol(%q) = %v, want %v", test.transport, got, *test.want)
			}
		})
	}
}

func TestBackChannelProtocolIsTCP(t *testing.T) {
	if got := backChannelProtocol(); got == nil || *got != gortsplib.ProtocolTCP {
		t.Fatalf("backChannelProtocol() = %v, want tcp", got)
	}
}

func TestCheckSecureUDP(t *testing.T) {
	tcp := gortsplib.ProtocolTCP
	udp := gortsplib.ProtocolUDP
	plainVideo := []*description.Media{
		{Type: description.MediaTypeVideo, Profile: headers.TransportProfileAVP},
	}
	srtpVideo := []*description.Media{
		{Type: description.MediaTypeVideo, Profile: headers.TransportProfileSAVP},
	}
	tests := []struct {
		name     string
		scheme   string
		protocol *gortsplib.Protocol
		medias   []*description.Media
		wantErr  bool
	}{
		{name: "rtsps udp plain video", scheme: "rtsps", protocol: &udp, medias: plainVideo, wantErr: true},
		{name: "rtsps udp srtp video", scheme: "rtsps", protocol: &udp, medias: srtpVideo},
		{name: "rtsps tcp", scheme: "rtsps", protocol: &tcp, medias: plainVideo},
		{name: "rtsps auto", scheme: "rtsps", protocol: nil, medias: plainVideo},
		{name: "rtsp udp", scheme: "rtsp", protocol: &udp, medias: plainVideo},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := checkSecureUDP(test.scheme, test.protocol, test.medias)
			if test.wantErr && !errors.Is(err, errSecureUDPUnavailable) {
				t.Fatalf("checkSecureUDP() = %v, want errSecureUDPUnavailable", err)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("checkSecureUDP() = %v, want nil", err)
			}
		})
	}
}

func TestUDPReadBufferSize(t *testing.T) {
	tcp := gortsplib.ProtocolTCP
	udp := gortsplib.ProtocolUDP
	tests := []struct {
		name     string
		protocol *gortsplib.Protocol
		rmemMax  int
		want     int
	}{
		{name: "tcp keeps os default", protocol: &tcp, rmemMax: 8 << 20, want: 0},
		{name: "unknown limit keeps os default", protocol: &udp, rmemMax: 0, want: 0},
		{name: "default linux limit", protocol: &udp, rmemMax: 212992, want: 212992},
		{name: "raised limit capped at target", protocol: &udp, rmemMax: 16 << 20, want: udpReadBufferTarget},
		{name: "auto uses target", protocol: nil, rmemMax: 16 << 20, want: udpReadBufferTarget},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := udpReadBufferSize(test.protocol, test.rmemMax); got != test.want {
				t.Fatalf("udpReadBufferSize() = %d, want %d", got, test.want)
			}
		})
	}
}
