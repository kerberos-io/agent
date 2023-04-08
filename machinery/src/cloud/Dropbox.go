// Package cloud contains the Dropbox implementation of the Cloud interface.
// It uses the Dropbox SDK to upload files to Dropbox.
package cloud

import (
	"bytes"
	"errors"
	"io"
	"os"

	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox"
	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox/files"
	"github.com/dropbox/dropbox-sdk-go-unofficial/v6/dropbox/users"
	"github.com/gin-gonic/gin"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/utils"
)

// UploadDropbox uploads the file to your Dropbox account using the access token and directory.
func UploadDropbox(configuration *models.Configuration, fileName string) (bool, bool, error) {

	config := configuration.Config
	token := config.Dropbox.AccessToken
	directory := config.Dropbox.Directory
	if directory != "" {
		// Check if trailing slash if not we'll add one.
		if directory[len(directory)-1:] != "/" {
			directory = directory + "/"
		}
	}

	if token == "" {
		err := "UploadDropbox: Dropbox not properly configured"
		log.Log.Info(err)
		return false, true, errors.New(err)
	}

	// Upload to Dropbox
	log.Log.Info("UploadDropbox: Uploading to Dropbox")
	log.Log.Info("UploadDropbox: Upload started for " + fileName)
	fullname := "data/recordings/" + fileName

	dConfig := dropbox.Config{
		Token:    token,
		LogLevel: dropbox.LogInfo, // if needed, set the desired logging level. Default is off
	}

	var fileReader io.Reader
	var err error
	// if encryption enabled, we will encrypt the file before uploading.
	if config.Encryption == "true" {
		// Encrypt the file
		log.Log.Info("UploadDropbox: Encrypting file")
		//file, err :=
		encryptedFile, err := utils.EncryptFileWithSharedKey(fullname, config.SharedKey)
		if err != nil {
			log.Log.Error("UploadDropbox: Error encrypting file: " + err.Error())
			return false, false, err
		}
		// Convert the encrypted file to a reader
		fileReader = bytes.NewReader(encryptedFile)
	} else {
		file, _ := os.OpenFile(fullname, os.O_RDWR, 0755)
		if file != nil {
			defer file.Close()
		}
		fileReader = file
	}

	if err == nil {
		// Upload the file
		dbf := files.New(dConfig)
		res, err := dbf.Upload(&files.UploadArg{
			CommitInfo: files.CommitInfo{
				Path: "/" + directory + fileName,
				Mode: &files.WriteMode{
					Tagged: dropbox.Tagged{
						Tag: "overwrite",
					},
				},
			},
		}, fileReader)

		if err != nil {
			log.Log.Error("UploadDropbox: Error uploading file: " + err.Error())
			return false, false, err
		}

		log.Log.Info("UploadDropbox: File uploaded successfully, " + res.Name)
		return true, true, nil
	}

	log.Log.Error("UploadDropbox: Error opening file: " + err.Error())
	return false, true, err
}

// VerifyDropbox verifies if the Dropbox token is valid and it is able to upload a file.
func VerifyDropbox(config models.Config, c *gin.Context) {

	token := config.Dropbox.AccessToken
	directory := config.Dropbox.Directory
	if directory != "" {
		// Check if trailing slash if not we'll add one.
		if directory[len(directory)-1:] != "/" {
			directory = directory + "/"
		}
	}

	if token != "" {
		dConfig := dropbox.Config{
			Token:    token,
			LogLevel: dropbox.LogInfo, // if needed, set the desired logging level. Default is off
		}
		dbx := users.New(dConfig)
		_, err := dbx.GetCurrentAccount()
		if err != nil {
			c.JSON(400, models.APIResponse{
				Data: "Something went wrong while reaching the Dropbox API: " + err.Error(),
			})
		} else {

			// Upload the file
			content := TestFile
			file := bytes.NewReader(content)

			dbf := files.New(dConfig)
			_, err := dbf.Upload(&files.UploadArg{
				CommitInfo: files.CommitInfo{
					Path: "/" + directory + "kerbers-agent-test.mp4",
					Mode: &files.WriteMode{
						Tagged: dropbox.Tagged{
							Tag: "overwrite",
						},
					},
				},
			}, file)

			if err != nil {
				c.JSON(400, models.APIResponse{
					Data: "Something went wrong while reaching the Dropbox API: " + err.Error(),
				})
			} else {
				c.JSON(200, models.APIResponse{
					Data: "Dropbox is working fine.",
				})
			}
		}
	} else {
		c.JSON(400, models.APIResponse{
			Data: "Dropbox token is not set.",
		})
	}
}
