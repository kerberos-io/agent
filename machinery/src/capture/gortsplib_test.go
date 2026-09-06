package capture

import (
	"bytes"
	"context"
	"encoding/pem"
	"errors"
	"math"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
