package conditions

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
)

const conditionHTTPTimeout = 10 * time.Second

var (
	conditionHTTPClient         = &http.Client{Timeout: conditionHTTPTimeout}
	conditionInsecureHTTPClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // #nosec G402 -- explicit operator opt-in
		},
		Timeout: conditionHTTPTimeout,
	}
)

func IsValidUriResponse(configuration *models.Configuration) (enabled bool) {
	config := configuration.Config
	conditionURI := config.ConditionURI
	enabled = true
	if conditionURI != "" {
		client := conditionHTTPClient
		if os.Getenv("AGENT_TLS_INSECURE") == "true" {
			client = conditionInsecureHTTPClient
		}

		payload := struct {
			CameraID   string `json:"camera_id"`
			CameraName string `json:"camera_name"`
			SiteID     string `json:"site_id"`
			HubKey     string `json:"hub_key"`
			Timestamp  string `json:"timestamp"`
		}{
			CameraID:   config.Key,
			CameraName: config.FriendlyName,
			SiteID:     config.HubSite,
			HubKey:     config.HubKey,
			Timestamp:  time.Now().Format("2006-01-02 15:04:05"),
		}
		jsonBody, err := json.Marshal(payload)
		if err != nil {
			log.Log.Error("conditions.uri.IsValidUriResponse(): failed to encode request: " + err.Error())
			return false
		}

		req, err := http.NewRequest(http.MethodPost, conditionURI, bytes.NewReader(jsonBody))
		if err != nil {
			log.Log.Error("conditions.uri.IsValidUriResponse(): failed to create request: " + err.Error())
			return false
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		if err == nil && resp != nil && resp.StatusCode == http.StatusOK {
			log.Log.Info("conditions.uri.IsValidUriResponse(): response 200, enabling recording.")
		} else {
			if err != nil {
				log.Log.Error("conditions.uri.IsValidUriResponse(): request failed: " + err.Error())
			} else {
				log.Log.Info(fmt.Sprintf("conditions.uri.IsValidUriResponse(): response %d, disabling recording.", resp.StatusCode))
			}
			enabled = false
		}
	}
	return
}
