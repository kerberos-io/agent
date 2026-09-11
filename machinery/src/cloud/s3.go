package cloud

import (
	"crypto/tls"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/minio/minio-go/v6"
	log "github.com/sirupsen/logrus"
)

func UploadS3(configuration *models.Configuration, fileName string) (bool, bool, error) {

	config := configuration.Config

	// timestamp_microseconds_instanceName_regionCoordinates_numberOfChanges_token
	// 1564859471_6-474162_oprit_577-283-727-375_1153_27.mp4
	// - Timestamp
	// - Size + - + microseconds
	// - device
	// - Region
	// - Number of changes
	// - Token

	if config.S3 == nil {
		errorMessage := "UploadS3: Uploading Failed, as no settings found"
		log.Error(errorMessage)
		return false, false, errors.New(errorMessage)
	}

	// Legacy support, should get rid of it!
	aws_access_key_id := config.S3.Publickey
	aws_secret_access_key := config.S3.Secretkey
	aws_region := config.S3.Region

	// This is the new way ;)
	if config.HubKey != "" {
		aws_access_key_id = config.HubKey
	}
	if config.HubPrivateKey != "" {
		aws_secret_access_key = config.HubPrivateKey
	}

	// Check if we have some credentials otherwise we abort the request.
	if aws_access_key_id == "" || aws_secret_access_key == "" {
		errorMessage := "UploadS3: Uploading Failed, as no credentials found"
		log.Error(errorMessage)
		return false, false, errors.New(errorMessage)
	}

	s3Client, err := minio.NewWithRegion("s3.amazonaws.com", aws_access_key_id, aws_secret_access_key, true, aws_region)
	if err != nil {
		errorMessage := "UploadS3: " + err.Error()
		log.Error(errorMessage)
		return false, true, errors.New(errorMessage)
	}

	// Check if we need to use the proxy.
	if config.S3.ProxyURI != "" {
		var transport http.RoundTripper = &http.Transport{
			Proxy: func(*http.Request) (*url.URL, error) {
				return url.Parse(config.S3.ProxyURI)
			},
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		s3Client.SetCustomTransport(transport)
	}

	recordingMetadata, ok := queuedRecordingMetadata(fileName)
	if !ok {
		recordingMetadata, ok = legacyS3RecordingMetadata(fileName, config.Key)
	}
	if !ok {
		errorMessage := "UploadS3: " + fileName + " is not a valid name."
		log.Error(errorMessage)
		return false, true, errors.New(errorMessage)
	}

	log.Info("UploadS3: Upload started for " + fileName)
	fullname := "data/recordings/" + fileName

	file, err := os.OpenFile(fullname, os.O_RDWR, 0755)
	if file != nil {
		defer file.Close()
	}

	if err != nil {
		errorMessage := "UploadS3: " + err.Error()
		log.Error(errorMessage)
		return false, true, errors.New(errorMessage)
	}

	fileInfo, err := file.Stat()
	if err != nil {
		errorMessage := "UploadS3: " + err.Error()
		log.Error(errorMessage)
		return false, true, errors.New(errorMessage)
	}

	n, err := s3Client.PutObject(config.S3.Bucket,
		config.S3.Username+"/"+fileName,
		file,
		fileInfo.Size(),
		minio.PutObjectOptions{
			ContentType:  "video/mp4",
			StorageClass: "ONEZONE_IA",
			UserMetadata: s3ObjectMetadata(recordingMetadata, config.Key, aws_access_key_id),
		})

	if err != nil {
		errorMessage := "UploadS3: Uploading Failed, " + err.Error()
		log.Error(errorMessage)
		return false, true, errors.New(errorMessage)
	} else {
		log.Info("UploadS3: Upload Finished, file has been uploaded to bucket: " + strconv.FormatInt(n, 10))
		return true, true, nil
	}
}

func s3ObjectMetadata(metadata models.RecordingUploadMetadata, deviceKey string, publicKey string) map[string]string {
	return map[string]string{
		"event-timestamp":         strconv.FormatInt(metadata.Timestamp/1000, 10),
		"event-microseconds":      strconv.FormatInt(metadata.Timestamp%1000, 10),
		"event-instancename":      metadata.DeviceName,
		"event-regioncoordinates": metadata.RegionCoordinates,
		"event-numberofchanges":   metadata.NumberOfChanges,
		"event-duration":          strconv.FormatUint(metadata.Duration, 10),
		"event-token":             strconv.FormatUint(metadata.Duration, 10),
		"productid":               deviceKey,
		"publickey":               publicKey,
		"uploadtime":              "now",
	}
}

func legacyS3RecordingMetadata(fileName string, deviceKey string) (models.RecordingUploadMetadata, bool) {
	fileParts := strings.Split(fileName, "_")
	if len(fileParts) < 6 {
		return models.RecordingUploadMetadata{}, false
	}
	seconds, secondsErr := strconv.ParseInt(fileParts[0], 10, 64)
	milliseconds := int64(0)
	precisionParts := strings.SplitN(fileParts[1], "-", 2)
	if len(precisionParts) == 2 {
		milliseconds, _ = strconv.ParseInt(precisionParts[1], 10, 64)
	}
	duration, durationErr := strconv.ParseUint(strings.TrimSuffix(fileParts[5], filepath.Ext(fileParts[5])), 10, 64)
	if secondsErr != nil || durationErr != nil || milliseconds < 0 || milliseconds >= 1000 {
		return models.RecordingUploadMetadata{}, false
	}
	return models.RecordingUploadMetadata{
		FileName:          fileName,
		DeviceKey:         deviceKey,
		DeviceName:        fileParts[2],
		Timestamp:         seconds*1000 + milliseconds,
		Duration:          duration,
		RegionCoordinates: fileParts[3],
		NumberOfChanges:   fileParts[4],
	}, true
}
