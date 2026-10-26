package http

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"
)

func TestDemoEnvironmentReadOnly(t *testing.T) {
	directory := t.TempDir()
	for name, contents := range map[string]string{
		"env.js": "production", "env.demo.js": "demo", "index.html": "index",
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0444); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(directory, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(directory, 0755) })
	router := gin.New()
	router.Use(demoEnvironmentMiddleware(filepath.Join(directory, "env.demo.js")))
	router.Use(static.Serve("/", static.LocalFile(directory, true)))
	for path, want := range map[string]string{"/env.js": "demo", "/": "index"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.String() != want {
			t.Fatalf("%s: status %d, body %q; want %q", path, response.Code, response.Body.String(), want)
		}
	}
	contents, err := os.ReadFile(filepath.Join(directory, "env.js"))
	if err != nil || string(contents) != "production" {
		t.Fatalf("production environment was changed: %q, %v", contents, err)
	}
}
