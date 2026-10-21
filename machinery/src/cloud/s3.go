package cloud

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/utils"
	"github.com/minio/minio-go/v6"
	log "github.com/sirupsen/logrus"
)

func UploadS3(configuration *models.Configuration, fileName string) (bool, bool, error) {

	config := configuration.Config

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

	objectName, userMetadata, err := s3RecordingUploadDetails(config, fileName, aws_access_key_id)
	if err != nil {
		log.WithError(err).Error("UploadS3: invalid recording upload metadata")
		return false, true, err
	}

	log.Info("UploadS3: Upload started for " + fileName)
	file, err := utils.OpenRecording("data/recordings", fileName)
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
		objectName,
		file,
		fileInfo.Size(),
		minio.PutObjectOptions{
			ContentType:  "video/mp4",
			StorageClass: "ONEZONE_IA",
			UserMetadata: userMetadata,
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

func s3RecordingObjectName(username, deviceKey string, startRecording int64) string {
	return username + "/" + time.UnixMilli(startRecording).UTC().Format("2006/01/02") +
		"/" + strconv.FormatInt(startRecording, 10) + "-" + deviceKey + ".mp4"
}

func s3RecordingUploadDetails(config models.Config, fileName, publicKey string) (string, map[string]string, error) {
	value, err := readRecordingUploadMetadataFile(fileName)
	if err != nil && !os.IsNotExist(err) {
		return "", nil, err
	}

	deviceKey := config.Key
	var startRecording int64
	userMetadata := map[string]string{
		"publickey":  publicKey,
		"uploadtime": "now",
	}
	marker := strings.TrimSpace(string(value))
	_, fpsErr := strconv.ParseFloat(marker, 64)
	if err == nil && marker != "" && fpsErr != nil {
		var metadata models.RecordingUploadMetadata
		if err := json.Unmarshal(value, &metadata); err != nil {
			return "", nil, fmt.Errorf("decode recording upload metadata for %s: %w", fileName, err)
		}
		if metadata.FileName != "" && metadata.FileName != filepath.Base(fileName) {
			return "", nil, fmt.Errorf("recording upload metadata filename does not match %s", fileName)
		}
		if metadata.Timestamp <= 0 {
			return "", nil, fmt.Errorf("invalid recording timestamp in upload metadata for %s", fileName)
		}
		deviceKey = metadata.DeviceKey
		startRecording = metadata.Timestamp
		userMetadata["event-instancename"] = config.Name
		if metadata.DeviceName != "" {
			userMetadata["event-instancename"] = metadata.DeviceName
		}
		if metadata.RegionCoordinates != "" {
			userMetadata["event-regioncoordinates"] = metadata.RegionCoordinates
		}
		if metadata.NumberOfChanges != "" {
			userMetadata["event-numberofchanges"] = metadata.NumberOfChanges
		}
		userMetadata["event-microseconds"] = strconv.FormatInt(metadata.Timestamp%1000*1000, 10)
		userMetadata["duration"] = strconv.FormatUint(metadata.Duration, 10)
		userMetadata["encrypted"] = strconv.FormatBool(metadata.Encrypted)
		if metadata.FPS > 0 && metadata.FPS <= 240 {
			userMetadata["fps"] = strconv.FormatFloat(metadata.FPS, 'f', -1, 64)
		}
	} else {
		// Empty and FPS-only markers predate structured recording metadata.
		fileParts := strings.Split(fileName, "_")
		if len(fileParts) < 6 {
			return "", nil, fmt.Errorf("%s is not a valid legacy recording name", fileName)
		}
		startRecording, err = strconv.ParseInt(fileParts[0], 10, 64)
		if err != nil {
			return "", nil, fmt.Errorf("invalid recording timestamp in %s: %w", fileName, err)
		}
		if startRecording <= 0 || startRecording > math.MaxInt64/1000 {
			return "", nil, fmt.Errorf("invalid recording timestamp in %s", fileName)
		}
		startRecording *= 1000
		_, precision, found := strings.Cut(fileParts[1], "-")
		if found {
			fraction, err := strconv.ParseInt(precision, 10, 64)
			if err != nil || fraction < 0 || fraction >= 1000000 {
				return "", nil, fmt.Errorf("invalid recording precision in %s", fileName)
			}
			if len(precision) > 3 {
				fraction /= 1000
			}
			startRecording += fraction
		}
		token, _ := strconv.Atoi(fileParts[5])
		userMetadata["event-microseconds"] = deviceKey
		userMetadata["event-instancename"] = fileParts[2]
		userMetadata["event-regioncoordinates"] = fileParts[3]
		userMetadata["event-numberofchanges"] = deviceKey
		userMetadata["event-token"] = strconv.Itoa(token)
	}
	if deviceKey == "" || strings.ContainsAny(deviceKey, "/\\") {
		return "", nil, errors.New("missing or invalid recording device key")
	}
	if startRecording <= 0 {
		return "", nil, fmt.Errorf("invalid recording timestamp for %s", fileName)
	}
	userMetadata["event-timestamp"] = strconv.FormatInt(startRecording/1000, 10)
	userMetadata["productid"] = deviceKey
	return s3RecordingObjectName(config.S3.Username, deviceKey, startRecording), userMetadata, nil
}
