package cloud

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/kerberos-io/agent/machinery/src/models"
	log "github.com/sirupsen/logrus"
)

func UploadKerberosHub(configuration *models.Configuration, fileName string) (bool, bool, error) {
	config := configuration.Config

	if config.HubURI == "" ||
		config.HubKey == "" ||
		config.HubPrivateKey == "" ||
		config.S3.Region == "" {
		err := "UploadKerberosHub: Kerberos Hub not properly configured."
		log.Info(err)
		return false, false, errors.New(err)
	}

	// timestamp_microseconds_instanceName_regionCoordinates_numberOfChanges_token
	// 1564859471_6-474162_oprit_577-283-727-375_1153_27.mp4
	// - Timestamp
	// - Size + - + microseconds
	// - device
	// - Region
	// - Number of changes
	// - Token

	log.WithFields(log.Fields{
		"component": "kerberos_hub",
		"event":     "upload_started",
	}).Info("Uploading recording to Kerberos Hub")

	// Prefer the resumable (tus) upload when enabled (the default). Kerberos Hub
	// authenticates the agent with its Hub public/private key and proxies the
	// resumable upload to the Kerberos Vault. When Hub does not expose a tus
	// endpoint (older deployments) we transparently fall back to the legacy
	// single-POST upload below.
	if resumableUploadsEnabled() {
		uploaded, _, supported, body, rerr := uploadHubResumable(&config, fileName, "UploadKerberosHub", "hub")
		if supported {
			if uploaded {
				log.WithFields(log.Fields{
					"component":      "kerberos_hub",
					"event":          "upload_completed",
					"response_bytes": len(body),
					"transport":      "tus",
				}).Info("Hub upload completed")
				return true, true, nil
			}
			if rerr != nil {
				log.WithError(rerr).WithFields(log.Fields{
					"component": "kerberos_hub",
					"event":     "upload_failed",
					"transport": "tus",
				}).Error("Hub upload failed")
			} else {
				log.WithFields(log.Fields{
					"component":      "kerberos_hub",
					"event":          "upload_incomplete",
					"response_bytes": len(body),
					"transport":      "tus",
				}).Warn("Hub upload incomplete")
			}
			return false, true, rerr
		}
		log.WithFields(log.Fields{
			"component": "kerberos_hub",
			"event":     "upload_transport_fallback",
			"transport": "http",
		}).Info("Resumable Hub upload unavailable; using legacy upload")
	}

	fullname := "data/recordings/" + fileName

	// Check if we still have the file otherwise we abort the request.
	file, err := os.OpenFile(fullname, os.O_RDWR, 0755)
	if file != nil {
		defer file.Close()
	}
	if err != nil {
		err := "UploadKerberosHub: Upload Failed, file doesn't exists anymore."
		log.Info(err)
		return false, false, errors.New(err)
	}

	// Check if we are allowed to upload to the hub with these credentials.
	// There might be different reasons like (muted, read-only..)
	req, err := http.NewRequest("HEAD", config.HubURI+"/storage/upload", nil)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"component": "kerberos_hub",
			"event":     "authorization_request_creation_failed",
		}).Error("Failed to create Hub upload authorization request")
		return false, true, fmt.Errorf("create Hub upload authorization request: %w", err)
	}

	req.Header.Set("X-Kerberos-Storage-FileName", fileName)
	req.Header.Set("X-Kerberos-Storage-Capture", "IPCamera")
	req.Header.Set("X-Kerberos-Storage-Device", config.Key)
	req.Header.Set("X-Kerberos-Hub-PublicKey", config.HubKey)
	req.Header.Set("X-Kerberos-Hub-PrivateKey", config.HubPrivateKey)
	req.Header.Set("X-Kerberos-Hub-Region", config.S3.Region)
	setQueuedRecordingMetadataHeaders(req.Header, fileName)

	var client *http.Client
	if os.Getenv("AGENT_TLS_INSECURE") == "true" {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client = &http.Client{Transport: tr, CheckRedirect: stripHubCredentialsOnCrossHostRedirect}
	} else {
		client = &http.Client{CheckRedirect: stripHubCredentialsOnCrossHostRedirect}
	}

	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}

	if err == nil && resp != nil {
		if resp.StatusCode == 200 {
			log.WithFields(log.Fields{
				"component":   "kerberos_hub",
				"event":       "upload_authorized",
				"status_code": resp.StatusCode,
			}).Debug("Hub upload authorized")
		} else {
			log.WithFields(log.Fields{
				"component":   "kerberos_hub",
				"event":       "upload_rejected",
				"status_code": resp.StatusCode,
			}).Warn("Hub upload rejected")
			return false, true, nil
		}
	}

	// Now we know we are allowed to upload to the hub, we can start uploading.
	req, err = http.NewRequest("POST", config.HubURI+"/storage/upload", file)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"component": "kerberos_hub",
			"event":     "upload_request_creation_failed",
		}).Error("Failed to create Hub upload request")
		return false, true, fmt.Errorf("create Hub upload request: %w", err)
	}
	req.Header.Set("Content-Type", "video/mp4")
	req.Header.Set("X-Kerberos-Storage-FileName", fileName)
	req.Header.Set("X-Kerberos-Storage-Capture", "IPCamera")
	req.Header.Set("X-Kerberos-Storage-Device", config.Key)
	req.Header.Set("X-Kerberos-Hub-PublicKey", config.HubKey)
	req.Header.Set("X-Kerberos-Hub-PrivateKey", config.HubPrivateKey)
	req.Header.Set("X-Kerberos-Hub-Region", config.S3.Region)
	setQueuedRecordingMetadataHeaders(req.Header, fileName)
	resp, err = client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}

	if err == nil {
		if resp != nil {
			body, err := ioutil.ReadAll(resp.Body)
			if err == nil {
				if resp.StatusCode == 200 {
					log.WithFields(log.Fields{
						"component":   "kerberos_hub",
						"event":       "upload_completed",
						"status_code": resp.StatusCode,
						"transport":   "http",
					}).Info("Hub upload completed")
					return true, true, nil
				} else {
					log.WithFields(log.Fields{
						"component":      "kerberos_hub",
						"event":          "upload_rejected",
						"response_bytes": len(body),
						"status_code":    resp.StatusCode,
						"transport":      "http",
					}).Warn("Hub upload rejected")
					return false, true, nil
				}
			}
		}
	}

	if err == nil {
		err = errors.New("Hub upload failed without a response")
	}
	log.WithError(err).WithFields(log.Fields{
		"component": "kerberos_hub",
		"event":     "upload_failed",
		"transport": "http",
	}).Error("Hub upload failed")
	return false, true, fmt.Errorf("Hub upload failed: %w", err)
}

// stripHubCredentialsOnCrossHostRedirect removes the custom Kerberos Hub
// credential headers on a redirect that crosses to a different host. net/http
// already strips the standard sensitive headers (Authorization, Cookie,
// WWW-Authenticate) on a cross-host redirect, but it does NOT strip
// custom-named headers, so without this the Hub private/public keys would be
// forwarded to any host the configured HubURI redirects to.
func stripHubCredentialsOnCrossHostRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return nil
	}
	if req.URL.Host != via[0].URL.Host {
		req.Header.Del("X-Kerberos-Hub-PrivateKey")
		req.Header.Del("X-Kerberos-Hub-PublicKey")
	}
	return nil
}
