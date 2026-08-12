package capture

import "testing"

func TestRTSPSTLSConfig(t *testing.T) {
	t.Run("verifies certificates by default", func(t *testing.T) {
		t.Setenv(rtspsInsecureEnv, "")

		if got := rtspsTLSConfig(); got != nil {
			t.Fatalf("rtspsTLSConfig() = %#v, want nil", got)
		}
	})

	t.Run("allows explicit insecure mode", func(t *testing.T) {
		t.Setenv(rtspsInsecureEnv, "true")

		got := rtspsTLSConfig()
		if got == nil || !got.InsecureSkipVerify {
			t.Fatalf("rtspsTLSConfig() = %#v, want InsecureSkipVerify enabled", got)
		}
	})
}
