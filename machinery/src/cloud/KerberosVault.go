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

func UploadKerberosVault(configuration *models.Configuration, fileName string) (bool, bool, error) {

	config := configuration.Config

	if config.KStorage.AccessKey == "" ||
		config.KStorage.SecretAccessKey == "" ||
		config.KStorage.Directory == "" ||
		config.KStorage.URI == "" {
		err := "UploadKerberosVault: Kerberos Vault not properly configured."
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
	fullname := "data/recordings/" + fileName

	file, err := os.OpenFile(fullname, os.O_RDWR, 0755)
	if file != nil {
		defer file.Close()
	}
	if err != nil {
		err := "UploadKerberosVault: Upload Failed, file doesn't exists anymore."
		log.Log.Info(err)
		return false, true, errors.New(err)
	}

	publicKey := config.KStorage.CloudKey
	// This is the new way ;)
	if config.HubKey != "" {
		publicKey = config.HubKey
	}

	req, err := http.NewRequest("POST", config.KStorage.URI+"/storage", file)
	if err != nil {
		errorMessage := "UploadKerberosVault: error reading request, " + config.KStorage.URI + "/storage: " + err.Error()
		log.Log.Error(errorMessage)
		return false, true, errors.New(errorMessage)
	}
	req.Header.Set("Content-Type", "video/mp4")
	req.Header.Set("X-Kerberos-Storage-CloudKey", publicKey)
	req.Header.Set("X-Kerberos-Storage-AccessKey", config.KStorage.AccessKey)
	req.Header.Set("X-Kerberos-Storage-SecretAccessKey", config.KStorage.SecretAccessKey)
	req.Header.Set("X-Kerberos-Storage-Provider", config.KStorage.Provider)
	req.Header.Set("X-Kerberos-Storage-FileName", fileName)
	req.Header.Set("X-Kerberos-Storage-Device", config.Key)
	req.Header.Set("X-Kerberos-Storage-Capture", "IPCamera")
	req.Header.Set("X-Kerberos-Storage-Directory", config.KStorage.Directory)

	var client *http.Client
	if os.Getenv("AGENT_TLS_INSECURE") == "true" {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client = &http.Client{Transport: tr}
	} else {
		client = &http.Client{}
	}

	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}

	if err == nil {
		if resp != nil {
			body, err := ioutil.ReadAll(resp.Body)
			if err == nil {
				if resp.StatusCode == 200 {
					log.Log.Info("UploadKerberosVault: Upload Finished, " + resp.Status + ", " + string(body))
					return true, true, nil
				} else {
					log.Log.Info("UploadKerberosVault: Upload Failed, " + resp.Status + ", " + string(body))
					return false, true, nil
				}
			}
		}
	}

	errorMessage := "UploadKerberosVault: Upload Failed, " + err.Error()
	log.Log.Info(errorMessage)
	return false, true, errors.New(errorMessage)
}
