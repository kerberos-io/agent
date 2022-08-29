package cloud

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/minio/minio-go/v6"
)

func UploadS3(configuration *models.Configuration, fileName string, directory string) bool {

	config := configuration.Config

	//fmt.Println("Uploading...")
	// timestamp_microseconds_instanceName_regionCoordinates_numberOfChanges_token
	// 1564859471_6-474162_oprit_577-283-727-375_1153_27.mp4
	// - Timestamp
	// - Size + - + microseconds
	// - device
	// - Region
	// - Number of changes
	// - Token

	if config.S3 == nil {
		log.Log.Error("UploadS3: Uploading Failed, as no settings found")
		return false
	}

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

	s3Client, err := minio.NewWithRegion("s3.amazonaws.com", aws_access_key_id, aws_secret_access_key, true, aws_region)
	if err != nil {
		log.Log.Error(err.Error())
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

	fileParts := strings.Split(fileName, "_")
	if len(fileParts) == 1 {
		log.Log.Error("ERROR: " + fileName + " is not a valid name.")
		os.Remove(directory + "/" + fileName)
		return false
	}

	deviceKey := config.Key
	startRecording, _ := strconv.ParseInt(fileParts[0], 10, 64)
	devicename := fileParts[2]
	coordinates := fileParts[3]
	//numberOfChanges := fileParts[4]
	token, _ := strconv.Atoi(fileParts[5])

	log.Log.Info("UploadS3: Upload started for " + fileName)
	fullname := "data/recordings/" + fileName

	file, err := os.OpenFile(fullname, os.O_RDWR, 0755)
	defer file.Close()
	if err != nil {
		log.Log.Error("UploadS3: " + err.Error())
		os.Remove(directory + "/" + fileName)
		return false
	}

	fileInfo, err := file.Stat()
	if err != nil {
		log.Log.Error("UploadS3: " + err.Error())
		os.Remove(directory + "/" + fileName)
		return false
	}

	n, err := s3Client.PutObject(config.S3.Bucket,
		config.S3.Username+"/"+fileName,
		file,
		fileInfo.Size(),
		minio.PutObjectOptions{
			ContentType:  "video/mp4",
			StorageClass: "ONEZONE_IA",
			UserMetadata: map[string]string{
				"event-timestamp":         strconv.FormatInt(startRecording, 10),
				"event-microseconds":      deviceKey,
				"event-instancename":      devicename,
				"event-regioncoordinates": coordinates,
				"event-numberofchanges":   deviceKey,
				"event-token":             strconv.Itoa(token),
				"productid":               deviceKey,
				"publickey":               aws_access_key_id,
				"uploadtime":              "now",
			},
		})

	if err != nil {
		log.Log.Error("UploadS3: Uploading Failed, " + err.Error())
		return false
	} else {
		log.Log.Info("UploadS3: Upload Finished, file has been uploaded to bucket: " + strconv.FormatInt(n, 10))
		os.Remove(directory + "/" + fileName)
		return true
	}
}
