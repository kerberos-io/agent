package http

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestUIEnvironmentIsServedWithoutChangingAssets(t *testing.T) {
	for _, mode := range []string{"", "demo"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("AGENT_MODE", mode)
			directory := t.TempDir()
			www := filepath.Join(directory, "www")
			if err := os.Mkdir(www, 0755); err != nil {
				t.Fatal(err)
			}
			for name, content := range map[string]string{"env.js": "normal", "env.demo.js": "demo"} {
				if err := os.WriteFile(filepath.Join(www, name), []byte(content), 0444); err != nil {
					t.Fatal(err)
				}
			}
			router := gin.New()
			if err := addUIEnvironmentRoute(router, directory); err != nil {
				t.Fatal(err)
			}
			want := "normal"
			if mode == "demo" {
				want = "demo"
			}
			for _, method := range []string{http.MethodGet, http.MethodHead} {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(method, "/env.js", nil))
				if response.Code != http.StatusOK {
					t.Fatalf("%s /env.js: status %d", method, response.Code)
				}
				if method == http.MethodGet && response.Body.String() != want {
					t.Fatalf("environment = %q, want %q", response.Body.String(), want)
				}
				if method == http.MethodHead && response.Body.Len() != 0 {
					t.Fatal("HEAD returned a response body")
				}
			}
			for name, want := range map[string]string{"env.js": "normal", "env.demo.js": "demo"} {
				got, err := os.ReadFile(filepath.Join(www, name))
				if err != nil || string(got) != want {
					t.Fatalf("asset %s changed: %q, %v", name, got, err)
				}
			}
		})
	}
}

func TestUIEnvironmentRejectsMissingAsset(t *testing.T) {
	t.Setenv("AGENT_MODE", "demo")
	if err := addUIEnvironmentRoute(gin.New(), t.TempDir()); err == nil {
		t.Fatal("missing demo environment was accepted")
	}
}
