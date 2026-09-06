package conditions

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/kerberos-io/agent/machinery/src/models"
	log "github.com/sirupsen/logrus"
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
			log.WithError(err).WithFields(log.Fields{
				"component": "conditions/uri",
				"event":     "request_encoding_failed",
			}).Error("Failed to encode condition request")
			return false
		}

		req, err := http.NewRequest(http.MethodPost, conditionURI, bytes.NewReader(jsonBody))
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"component": "conditions/uri",
				"event":     "request_creation_failed",
			}).Error("Failed to create condition request")
			return false
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		if err == nil && resp != nil && resp.StatusCode == http.StatusOK {
			log.WithFields(log.Fields{
				"component":   "conditions/uri",
				"event":       "recording_enabled",
				"status_code": resp.StatusCode,
			}).Info("Condition request enabled recording")
		} else {
			if err != nil {
				log.WithError(err).WithFields(log.Fields{
					"component": "conditions/uri",
					"event":     "request_failed",
				}).Error("Condition request failed")
			} else {
				log.WithFields(log.Fields{
					"component":   "conditions/uri",
					"event":       "recording_disabled",
					"status_code": resp.StatusCode,
				}).Info("Condition request disabled recording")
			}
			enabled = false
		}
	}
	return
}
