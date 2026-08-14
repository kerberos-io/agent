package capture

import (
	"bytes"
	"encoding/pem"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

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
