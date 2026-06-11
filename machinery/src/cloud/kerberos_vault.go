package cloud

import (
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
)

// We will count the number of retries we have done.
// If we have done more than "kstorageRetryPolicy" retries, we will stop, and start sending to the secondary storage.
var kstorageRetryCount = 0
var kstorageRetryTimeout = time.Now().Unix()

func UploadKerberosVault(configuration *models.Configuration, fileName string) (bool, bool, error) {

	config := configuration.Config

	if config.KStorage.AccessKey == "" ||
		config.KStorage.SecretAccessKey == "" ||
		config.KStorage.Directory == "" ||
		config.KStorage.URI == "" {
		err := "UploadKerberosVault: Kerberos Vault not properly configured"
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
	// KerberosCloud, this means storage is disabled and proxy enabled.
	log.Log.Info("UploadKerberosVault: Uploading to Kerberos Vault (" + config.KStorage.URI + ")")
	log.Log.Info("UploadKerberosVault: Upload started for " + fileName)

	publicKey := config.KStorage.CloudKey
	if config.HubKey != "" {
		publicKey = config.HubKey
	}

	// We need to check if we are in a retry timeout.
	if kstorageRetryTimeout <= time.Now().Unix() {
		uploaded, responded, body, err := sendToVault(*config.KStorage, publicKey, config.Key, fileName, "UploadKerberosVault", "primary")
		if uploaded {
			kstorageRetryCount = 0
			log.Log.Info("UploadKerberosVault: Upload Finished, " + body)
			return true, true, nil
		}

		if err != nil {
			log.Log.Info("UploadKerberosVault: Upload Failed, " + err.Error())
		} else {
			log.Log.Info("UploadKerberosVault: Upload Failed, " + body)
		}

		// We only advance the retry policy when the vault gave a definitive
		// response (mirroring the original behaviour where transient network
		// errors did not consume retries). When the retry count reaches the
		// configured maximum we back off for the configured timeout.
		if responded {
			if kstorageRetryCount < config.KStorage.MaxRetries {
				kstorageRetryCount = (kstorageRetryCount + 1)
			}
			if kstorageRetryCount == config.KStorage.MaxRetries {
				kstorageRetryTimeout = time.Now().Add(time.Duration(config.KStorage.Timeout) * time.Second).Unix()
			}
		}
	}

	// We might need to check if we can upload to our secondary storage.
	if config.KStorageSecondary.AccessKey == "" ||
		config.KStorageSecondary.SecretAccessKey == "" ||
		config.KStorageSecondary.Directory == "" ||
		config.KStorageSecondary.URI == "" {
		log.Log.Info("UploadKerberosVault (Secondary): Secondary Kerberos Vault not properly configured.")
	} else {

		if kstorageRetryCount < config.KStorage.MaxRetries {
			log.Log.Info("UploadKerberosVault (Secondary): Do not upload to secondary storage, we are still in retry policy.")
			return false, true, nil
		}

		log.Log.Info("UploadKerberosVault (Secondary): Uploading to Secondary Kerberos Vault (" + config.KStorageSecondary.URI + ")")

		uploaded, _, body, err := sendToVault(*config.KStorageSecondary, publicKey, config.Key, fileName, "UploadKerberosVault (Secondary)", "secondary")
		if uploaded {
			log.Log.Info("UploadKerberosVault (Secondary): Upload Finished to secondary, " + body)
			return true, true, nil
		}

		if err != nil {
			log.Log.Info("UploadKerberosVault (Secondary): Upload Failed to secondary, " + err.Error())
		} else {
			log.Log.Info("UploadKerberosVault (Secondary): Upload Failed to secondary, " + body)
		}
	}

	return false, true, nil
}

// sendToVault uploads a single recording to one Kerberos Vault. When resumable
// uploads are enabled (the default) it attempts the tus protocol first and, if
// the vault does not expose a tus endpoint (older deployments), transparently
// falls back to the legacy single-shot POST.
//
// It returns whether the upload succeeded, whether the vault gave a definitive
// HTTP response (so the caller can advance its retry policy), a short message
// for logging, and a transport error if any.
func sendToVault(vault models.KStorage, publicKey, deviceKey, fileName, label, slot string) (bool, bool, string, error) {
	if resumableUploadsEnabled() {
		uploaded, responded, supported, body, err := uploadVaultResumable(vault, publicKey, deviceKey, fileName, label, slot)
		if supported {
			return uploaded, responded, body, err
		}
		log.Log.Info(label + ": resumable (tus) endpoint not available, falling back to legacy upload")
	}
	return uploadVaultLegacy(vault, publicKey, deviceKey, fileName, label)
}

// uploadVaultLegacy performs the original single-request upload: the whole file
// is sent as the body of a POST to {URI}/storage. Kept for backwards
// compatibility with vault deployments that do not support resumable uploads.
func uploadVaultLegacy(vault models.KStorage, publicKey, deviceKey, fileName, label string) (bool, bool, string, error) {
	fullname := "data/recordings/" + fileName

	file, err := os.OpenFile(fullname, os.O_RDWR, 0755)
	if file != nil {
		defer file.Close()
	}
	if err != nil {
		msg := label + ": Upload Failed, file doesn't exists anymore"
		log.Log.Info(msg)
		return false, false, "", errors.New(msg)
	}

	req, err := http.NewRequest("POST", vault.URI+"/storage", file)
	if err != nil {
		errorMessage := label + ": error reading request, " + vault.URI + "/storage: " + err.Error()
		log.Log.Error(errorMessage)
		return false, false, "", errors.New(errorMessage)
	}
	req.Header.Set("Content-Type", "video/mp4")
	setVaultHeaders(req.Header, vault, publicKey, deviceKey, fileName)

	client := newVaultHTTPClient(0)
	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return false, false, "", err
	}

	body, rerr := io.ReadAll(resp.Body)
	if rerr != nil {
		return false, false, "", rerr
	}

	if resp.StatusCode == 200 {
		return true, true, resp.Status + ", " + string(body), nil
	}
	return false, true, resp.Status + ", " + string(body), nil
}

// setVaultHeaders sets the standard Kerberos Vault headers used by the legacy
// single-POST upload.
func setVaultHeaders(h http.Header, vault models.KStorage, publicKey, deviceKey, fileName string) {
	h.Set("X-Kerberos-Storage-CloudKey", publicKey)
	h.Set("X-Kerberos-Storage-AccessKey", vault.AccessKey)
	h.Set("X-Kerberos-Storage-SecretAccessKey", vault.SecretAccessKey)
	h.Set("X-Kerberos-Storage-Provider", vault.Provider)
	h.Set("X-Kerberos-Storage-FileName", fileName)
	h.Set("X-Kerberos-Storage-Device", deviceKey)
	h.Set("X-Kerberos-Storage-Capture", "IPCamera")
	h.Set("X-Kerberos-Storage-Directory", vault.Directory)
}

// newVaultHTTPClient builds an HTTP client honouring the AGENT_TLS_INSECURE
// escape hatch. A timeout of 0 disables the client-level timeout, which is
// required for streaming large upload bodies.
func newVaultHTTPClient(timeout time.Duration) *http.Client {
	client := &http.Client{}
	if os.Getenv("AGENT_TLS_INSECURE") == "true" {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}
	if timeout > 0 {
		client.Timeout = timeout
	}
	return client
}
