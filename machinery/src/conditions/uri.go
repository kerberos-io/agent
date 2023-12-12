package conditions

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
)

func IsValidUriResponse(configuration *models.Configuration) (enabled bool) {
	config := configuration.Config
	conditionURI := config.ConditionURI
	enabled = true
	if conditionURI != "" {

		// We will send a POST request to the conditionURI, and expect a 200 response.
		// In the payload we will send some information, so the other end can decide
		// if it should enable or disable recording.

		var client *http.Client
		if os.Getenv("AGENT_TLS_INSECURE") == "true" {
			tr := &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}
			client = &http.Client{Transport: tr}
		} else {
			client = &http.Client{}
		}

		var object = fmt.Sprintf(`{
			"camera_id" : "%s",
			"camera_name" : "%s",
			"site_id" : "%s",
			"hub_key" : "%s",
			"timestamp" : "%s",
		}`, config.Key, config.FriendlyName, config.HubSite, config.HubKey, time.Now().Format("2006-01-02 15:04:05"))

		var jsonStr = []byte(object)
		buffy := bytes.NewBuffer(jsonStr)
		req, _ := http.NewRequest("POST", conditionURI, buffy)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if resp != nil {
			resp.Body.Close()
		}
		if err == nil && resp.StatusCode == 200 {
			log.Log.Info("conditions.uri.IsValidUriResponse(): response 200, enabling recording.")
		} else {
			log.Log.Info("conditions.uri.IsValidUriResponse(): response not 200, disabling recording.")
			enabled = false
		}
	}
	return
}
