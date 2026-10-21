package http

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kerberos-io/agent/machinery/src/encryption"
	"github.com/kerberos-io/agent/machinery/src/models"
)

func TestFilesServesFlatAndDatedRecordings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, layout := range []string{"flat", "dated"} {
		for _, encrypted := range []bool{false, true} {
			name := layout + "-plain"
			if encrypted {
				name = layout + "-encrypted"
			}
			t.Run(name, func(t *testing.T) {
				const fileName = "1767355200123-device-key.mp4"
				directory := t.TempDir()
				relativePath := fileName
				if layout == "dated" {
					relativePath = "2026/01/02/" + fileName
				}
				filePath := filepath.Join(directory, "data/recordings", relativePath)
				if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
					t.Fatal(err)
				}
				config := &models.Configuration{Config: models.Config{Encryption: &models.Encryption{}}}
				content := []byte("recording-payload")
				if encrypted {
					config.Config.Encryption.Recordings = "true"
					config.Config.Encryption.SymmetricKey = "0123456789abcdef0123456789abcdef"
					ciphertext, err := encryption.AesEncrypt(content, config.Config.Encryption.SymmetricKey)
					if err != nil {
						t.Fatal(err)
					}
					content = []byte(ciphertext)
				}
				if err := os.WriteFile(filePath, content, 0o644); err != nil {
					t.Fatal(err)
				}
				router := gin.New()
				router.GET("/file/*filepath", func(c *gin.Context) { Files(c, directory, config) })
				for _, reference := range []string{relativePath, fileName} {
					response := httptest.NewRecorder()
					router.ServeHTTP(response, httptest.NewRequest("GET", "/file/"+reference, nil))
					if response.Code != 200 || response.Body.String() != "recording-payload" {
						t.Fatalf("GET %s = %d %q", reference, response.Code, response.Body.String())
					}
					if got := response.Header().Get("Content-Type"); got != "video/mp4" {
						t.Fatalf("Content-Type = %q, want video/mp4", got)
					}
				}
			})
		}
	}
}

func TestFilesRejectsMissingAndOutsideRecordings(t *testing.T) {
	directory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(directory, "data/recordings"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "data/outside.mp4"), []byte("not a recording"), 0o644); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.GET("/file/*filepath", func(c *gin.Context) { Files(c, directory, &models.Configuration{}) })
	for _, reference := range []string{"missing.mp4", "../outside.mp4", "2026/01/02/missing.mp4"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", "/file/"+reference, nil))
		if response.Code != 404 {
			t.Fatalf("GET %s = %d, want 404", reference, response.Code)
		}
	}
}
