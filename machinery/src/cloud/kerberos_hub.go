package cloud

import (
	"crypto/tls"
	"errors"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
)

func UploadKerberosHub(configuration *models.Configuration, fileName string) (bool, bool, error) {
	config := configuration.Config

	if config.HubURI == "" ||
		config.HubKey == "" ||
		config.HubPrivateKey == "" ||
		config.S3.Region == "" {
		err := "UploadKerberosHub: Kerberos Hub not properly configured."
		log.Log.Info(err)
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

	log.Log.Info("UploadKerberosHub: Uploading to Kerberos Hub (" + config.HubURI + ")")
	log.Log.Info("UploadKerberosHub: Upload started for " + fileName)

	// Prefer the resumable (tus) upload when enabled (the default). Kerberos Hub
	// authenticates the agent with its Hub public/private key and proxies the
	// resumable upload to the Kerberos Vault. When Hub does not expose a tus
	// endpoint (older deployments) we transparently fall back to the legacy
	// single-POST upload below.
	if resumableUploadsEnabled() {
		uploaded, _, supported, body, rerr := uploadHubResumable(&config, fileName, "UploadKerberosHub", "hub")
		if supported {
			if uploaded {
				log.Log.Info("UploadKerberosHub: Upload Finished (resumable), " + body)
				return true, true, nil
			}
			if rerr != nil {
				log.Log.Info("UploadKerberosHub: resumable upload failed, " + rerr.Error())
			} else {
				log.Log.Info("UploadKerberosHub: resumable upload incomplete, " + body)
			}
			return false, true, rerr
		}
		log.Log.Info("UploadKerberosHub: resumable (tus) endpoint not available, falling back to legacy upload")
	}

	fullname := "data/recordings/" + fileName

	// Check if we still have the file otherwise we abort the request.
	file, err := os.OpenFile(fullname, os.O_RDWR, 0755)
	if file != nil {
		defer file.Close()
	}
	if err != nil {
		err := "UploadKerberosHub: Upload Failed, file doesn't exists anymore."
		log.Log.Info(err)
		return false, false, errors.New(err)
	}

	// Check if we are allowed to upload to the hub with these credentials.
	// There might be different reasons like (muted, read-only..)
	req, err := http.NewRequest("HEAD", config.HubURI+"/storage/upload", nil)
	if err != nil {
		errorMessage := "UploadKerberosHub: error reading HEAD request, " + config.HubURI + "/storage: " + err.Error()
		log.Log.Error(errorMessage)
		return false, true, errors.New(errorMessage)
	}

	req.Header.Set("X-Kerberos-Storage-FileName", fileName)
	req.Header.Set("X-Kerberos-Storage-Capture", "IPCamera")
	req.Header.Set("X-Kerberos-Storage-Device", config.Key)
	req.Header.Set("X-Kerberos-Hub-PublicKey", config.HubKey)
	req.Header.Set("X-Kerberos-Hub-PrivateKey", config.HubPrivateKey)
	req.Header.Set("X-Kerberos-Hub-Region", config.S3.Region)

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

	if err == nil {
		if resp != nil {
			if err == nil {
				if resp.StatusCode == 200 {
					log.Log.Info("UploadKerberosHub: Upload allowed using the credentials provided (" + config.HubKey + ", " + config.HubPrivateKey + ")")
				} else {
					log.Log.Info("UploadKerberosHub: Upload NOT allowed using the credentials provided (" + config.HubKey + ", " + config.HubPrivateKey + ")")
					return false, true, nil
				}
			}
		}
	}

	// Now we know we are allowed to upload to the hub, we can start uploading.
	req, err = http.NewRequest("POST", config.HubURI+"/storage/upload", file)
	if err != nil {
		errorMessage := "UploadKerberosHub: error reading POST request, " + config.KStorage.URI + "/storage/upload: " + err.Error()
		log.Log.Error(errorMessage)
		return false, true, errors.New(errorMessage)
	}
	req.Header.Set("Content-Type", "video/mp4")
	req.Header.Set("X-Kerberos-Storage-FileName", fileName)
	req.Header.Set("X-Kerberos-Storage-Capture", "IPCamera")
	req.Header.Set("X-Kerberos-Storage-Device", config.Key)
	req.Header.Set("X-Kerberos-Hub-PublicKey", config.HubKey)
	req.Header.Set("X-Kerberos-Hub-PrivateKey", config.HubPrivateKey)
	req.Header.Set("X-Kerberos-Hub-Region", config.S3.Region)
	resp, err = client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}

	if err == nil {
		if resp != nil {
			body, err := ioutil.ReadAll(resp.Body)
			if err == nil {
				if resp.StatusCode == 200 {
					log.Log.Info("UploadKerberosHub: Upload Finished, " + resp.Status + ".")
					return true, true, nil
				} else {
					log.Log.Info("UploadKerberosHub: Upload Failed, " + resp.Status + ", " + string(body))
					return false, true, nil
				}
			}
		}
	}

	errorMessage := "UploadKerberosHub: Upload Failed, " + err.Error()
	log.Log.Info(errorMessage)
	return false, true, errors.New(errorMessage)
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
