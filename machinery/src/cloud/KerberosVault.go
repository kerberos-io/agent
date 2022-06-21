package cloud

import (
	"io/ioutil"
	"net/http"
	"os"

	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
)

func UploadKerberosVault(configuration *models.Configuration, fileName string, directory string) bool {

	config := configuration.Config

	if config.KStorage.AccessKey == "" ||
		config.KStorage.SecretAccessKey == "" ||
		config.KStorage.Provider == "" ||
		config.KStorage.Directory == "" ||
		config.KStorage.URI == "" {
		log.Log.Info("Kerberos Vault: not properly configured.")
	}

	//fmt.Println("Uploading...")
	// timestamp_microseconds_instanceName_regionCoordinates_numberOfChanges_token
	// 1564859471_6-474162_oprit_577-283-727-375_1153_27.mp4
	// - Timestamp
	// - Size + - + microseconds
	// - device
	// - Region
	// - Number of changes
	// - Token

	// KerberosCloud, this means storage is disabled and proxy enabled.
	log.Log.Info("Uploading to Kerberos Vault")

	log.Log.Info("Upload started for: " + fileName)
	fullname := "data/recordings/" + fileName

	file, err := os.OpenFile(fullname, os.O_RDWR, 0755)
	defer file.Close()
	if err != nil {
		log.Log.Info("Upload Failed: file doesn't exists anymore.")
		os.Remove(directory + "/" + fileName)
		return false
	}

	publicKey := config.KStorage.CloudKey
	// This is the new way ;)
	if config.HubKey != "" {
		publicKey = config.HubKey
	}

	log.Log.Info(config.KStorage.URI)
	req, err := http.NewRequest("POST", config.KStorage.URI+"/storage", file)
	if err != nil {
		log.Log.Error("Error reading request. " + err.Error())
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
	//client := &http.Client{Timeout: time.Second * 30}
	client := &http.Client{}

	resp, err := client.Do(req)

	if resp != nil {
		defer resp.Body.Close()
	}

	if err == nil {
		if resp != nil {
			body, err := ioutil.ReadAll(resp.Body)
			if err == nil {
				if resp.StatusCode == 200 {
					log.Log.Info("Upload Finished: " + resp.Status + ", " + string(body))
					// We will remove the file from disk as well
					os.Remove(fullname)
					os.Remove(directory + "/" + fileName)
				} else {
					log.Log.Info("Upload Failed: " + resp.Status + ", " + string(body))
				}
				resp.Body.Close()
			}
		}
	} else {
		log.Log.Info("Upload Failed: " + err.Error())
	}
	return true
}
