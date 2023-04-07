package cloud

import (
	"errors"
	"os"

	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox"
	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox/files"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
)

func UploadDropbox(configuration *models.Configuration, fileName string) (bool, bool, error) {

	config := configuration.Config
	token := config.Dropbox.AccessToken
	if token == "" {
		err := "UploadDropbox: Dropbox not properly configured."
		log.Log.Info(err)
		return false, false, errors.New(err)
	}

	// timestamp_microseconds_instanceName_regionCoordinates_numberOfChanges_token
	// 1564859471_6-474162_oprit_577-283-727-375_1153_27.mp4

	// Upload to Dropbox
	log.Log.Info("UploadDropbox: Uploading to Dropbox")
	log.Log.Info("UploadDropbox: Upload started for " + fileName)
	fullname := "data/recordings/" + fileName

	dConfig := dropbox.Config{
		Token:    token,
		LogLevel: dropbox.LogInfo, // if needed, set the desired logging level. Default is off
	}

	//dbx := users.New(dConfig)
	/*acc, err := dbx.GetAccount()
	if err != nil {
		log.Log.Error("UploadDropbox: Error getting account info: " + err.Error())
		return false, false, err
	}(/)*/

	file, err := os.OpenFile(fullname, os.O_RDWR, 0755)
	if file != nil {
		defer file.Close()
	}

	// Upload the file
	dbf := files.New(dConfig)
	res, err := dbf.Upload(&files.UploadArg{
		CommitInfo: files.CommitInfo{
			Path: "/" + fileName,
		},
	}, file)

	if err != nil {
		log.Log.Error("UploadDropbox: Error uploading file: " + err.Error())
		return false, false, err
	} else {
		log.Log.Info("UploadDropbox: File uploaded successfully, " + res.Name)
		return true, true, nil
	}
}
