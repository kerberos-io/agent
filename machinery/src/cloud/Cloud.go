package cloud

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/golang-module/carbon/v2"
	"github.com/kerberos-io/joy4/av/pubsub"
	"github.com/minio/minio-go/v6"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	av "github.com/kerberos-io/joy4/av"
	"github.com/kerberos-io/joy4/cgo/ffmpeg"
	"gocv.io/x/gocv"

	"net/http"
	"net/url"
	"runtime"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/kerberos-io/agent/machinery/src/computervision"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/onvif"
	"github.com/kerberos-io/agent/machinery/src/utils"
	"github.com/kerberos-io/agent/machinery/src/webrtc"
	"github.com/shirou/gopsutil/disk"
)

func PendingUpload() {
	ff, err := utils.ReadDirectory("./data/cloud/")
	if err == nil {
		for _, f := range ff {
			log.Log.Info(f.Name())
		}
	}
}

func HandleUpload(configuration *models.Configuration, communication *models.Communication) {

	log.Log.Debug("HandleUpload: started")

	config := configuration.Config
	watchDirectory := "./data/cloud/"

	if config.Offline == "true" {
		log.Log.Debug("HandleUpload: stopping as Offline is enabled.")
	} else {

	loop:
		for {
			ff, err := utils.ReadDirectory(watchDirectory)

			// This will check if we need to stop the thread,
			// because of a reconfiguration.
			select {
			case <-communication.HandleUpload:
				break loop
			case <-time.After(2 * time.Second):
			}

			if err == nil {
				for _, f := range ff {

					// This will check if we need to stop the thread,
					// because of a reconfiguration.
					select {
					case <-communication.HandleUpload:
						break loop
					default:
					}

					fileName := f.Name()
					if config.Cloud == "s3" {
						UploadS3(configuration, fileName, watchDirectory)
					} else if config.Cloud == "kstorage" {
						UploadKerberosVault(configuration, fileName, watchDirectory)
					}
				}
			}
		}
	}

	log.Log.Debug("HandleUpload: finished")
}

func HandleHeartBeat(configuration *models.Configuration, communication *models.Communication, uptimeStart time.Time) {
	log.Log.Debug("HandleHeartBeat: started")

	config := configuration.Config

	if config.Offline == "true" {
		log.Log.Debug("HandleHeartBeat: stopping as Offline is enabled.")
	} else {

		url := config.HeartbeatURI
		key := ""
		username := ""
		vaultURI := ""

		if config.Cloud == "s3" && config.S3 != nil && config.S3.Publickey != "" {
			username = config.S3.Username
			key = config.S3.Publickey
		} else if config.Cloud == "kstorage" && config.KStorage != nil && config.KStorage.CloudKey != "" {
			key = config.KStorage.CloudKey
			username = config.KStorage.Directory
			vaultURI = config.KStorage.URI
		}

		// This is the new way ;)
		if config.HubURI != "" {
			url = config.HubURI + "/devices/heartbeat"
		}
		if config.HubKey != "" {
			key = config.HubKey
		}

	loop:
		for {

			// We will formated the uptime to a human readable format
			// this will be used on Kerberos Hub: Uptime -> 1 day and 2 hours.
			uptimeFormatted := uptimeStart.Format("2006-01-02 15:04:05")
			uptimeString := carbon.Parse(uptimeFormatted).DiffForHumans()
			uptimeString = strings.ReplaceAll(uptimeString, "ago", "")

			usage, _ := disk.Usage("/")
			diskPercentUsed := strconv.Itoa(int(usage.UsedPercent))

			// We'll check which mode is enabled for the camera.
			onvifEnabled := "false"
			if config.Capture.IPCamera.ONVIFXAddr != "" {
				device, err := onvif.ConnectToOnvifDevice(configuration)
				if err == nil {
					capabilities := onvif.GetCapabilitiesFromDevice(device)
					for _, v := range capabilities {
						if v == "PTZ" || v == "ptz" {
							onvifEnabled = "true"
						}
					}
				}
			}

			// Check if the agent is running inside a cluster (Kerberos Factory) or as
			// an open source agent
			isEnterprise := false
			if os.Getenv("DEPLOYMENT") == "factory" || os.Getenv("MACHINERY_ENVIRONMENT") == "kubernetes" {
				isEnterprise = true
			}

			var object = fmt.Sprintf(`{
			"key" : "%s",
			"hash" : "826133658",
			"version" : "3.0.0",
			"cpuid" : "Serial: xxx",
			"clouduser" : "%s",
			"cloudpublickey" : "%s",
			"cameraname" : "%s",
			"cameratype" : "IPCamera",
			"docker" : true,
			"kios" : false,
			"raspberrypi" : false,
			"enterprise" : %t,
			"board" : "",
			"disk1size" : "%s",
			"disk3size" : "%s",
			"diskvdasize" :  "%s",
			"numberoffiles" : "33",
			"temperature" : "sh: 1: vcgencmd: not found",
			"wifissid" : "",
			"wifistrength" : "",
			"uptime" : "%s",
			"timestamp" : 1564747908,
			"siteID" : "%s",
			"onvif" : "%s"
		}`, config.Key, username, key, config.Name, isEnterprise, "0", "0", diskPercentUsed, uptimeString, config.HubSite, onvifEnabled)

			var jsonStr = []byte(object)
			buffy := bytes.NewBuffer(jsonStr)
			req, _ := http.NewRequest("POST", url, buffy)
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{}
			resp, err := client.Do(req)
			if resp != nil {
				resp.Body.Close()
			}
			if err == nil && resp.StatusCode == 200 {
				communication.CloudTimestamp.Store(time.Now().Unix())
				log.Log.Info("HandleHeartBeat: (200) Heartbeat received by Kerberos Hub.")
			} else {
				log.Log.Error("HandleHeartBeat: (400) Something went wrong while sending to Kerberos Hub.")
			}

			// If we have a vault connect, we will also send some analytics
			// to that service.
			if vaultURI != "" {
				buffy = bytes.NewBuffer(jsonStr)
				req, _ = http.NewRequest("POST", vaultURI+"/devices/heartbeat", buffy)
				req.Header.Set("Content-Type", "application/json")

				client = &http.Client{}
				resp, err = client.Do(req)
				if resp != nil {
					resp.Body.Close()
				}
				if err == nil && resp.StatusCode == 200 {
					log.Log.Info("HandleHeartBeat: (200) Heartbeat received by Kerberos Vault.")
				} else {
					log.Log.Error("HandleHeartBeat: (400) Something went wrong while sending to Kerberos Vault.")
				}
			}

			// This will check if we need to stop the thread,
			// because of a reconfiguration.
			select {
			case <-communication.HandleHeartBeat:
				break loop
			case <-time.After(15 * time.Second):
			}
		}
	}

	log.Log.Debug("HandleHeartBeat: finished")
}

func HandleLiveStreamSD(livestreamCursor *pubsub.QueueCursor, configuration *models.Configuration, communication *models.Communication, mqttClient mqtt.Client, decoder *ffmpeg.VideoDecoder, decoderMutex *sync.Mutex) {

	log.Log.Debug("HandleLiveStreamSD: started")

	config := configuration.Config

	if config.Offline == "true" {
		log.Log.Debug("HandleLiveStreamSD: stopping as Offline is enabled.")
	} else {

		key := ""
		if config.Cloud == "s3" && config.S3 != nil && config.S3.Publickey != "" {
			key = config.S3.Publickey
		} else if config.Cloud == "kstorage" && config.KStorage != nil && config.KStorage.CloudKey != "" {
			key = config.KStorage.CloudKey
		}
		// This is the new way ;)
		if config.HubKey != "" {
			key = config.HubKey
		}

		topic := "kerberos/" + key + "/device/" + config.Key + "/live"

		lastLivestreamRequest := int64(0)

		var cursorError error
		var pkt av.Packet

		for cursorError == nil {
			pkt, cursorError = livestreamCursor.ReadPacket()
			if len(pkt.Data) == 0 || !pkt.IsKeyFrame {
				continue
			}
			now := time.Now().Unix()
			select {
			case <-communication.HandleLiveSD:
				lastLivestreamRequest = now
			default:
			}
			if now-lastLivestreamRequest > 3 {
				continue
			}
			log.Log.Info("HandleLiveStreamSD: Sending base64 encoded images to MQTT.")
			sendImage(topic, mqttClient, pkt, decoder, decoderMutex)
		}
	}

	log.Log.Debug("HandleLiveStreamSD: finished")
}

func sendImage(topic string, mqttClient mqtt.Client, pkt av.Packet, decoder *ffmpeg.VideoDecoder, decoderMutex *sync.Mutex) {
	mat := computervision.GetRGBImage(pkt, decoder, decoderMutex)
	buffer, err := gocv.IMEncode(gocv.JPEGFileExt, mat)
	mat.Close()
	if err == nil {
		encoded := base64.StdEncoding.EncodeToString(buffer.GetBytes())
		mqttClient.Publish(topic, 0, false, encoded)
	}
	runtime.GC()
	debug.FreeOSMemory()
}

func HandleLiveStreamHD(livestreamCursor *pubsub.QueueCursor, configuration *models.Configuration, communication *models.Communication, mqttClient mqtt.Client, codecs []av.CodecData, decoder *ffmpeg.VideoDecoder, decoderMutex *sync.Mutex) {

	config := configuration.Config

	if config.Offline == "true" {
		log.Log.Debug("HandleLiveStreamHD: stopping as Offline is enabled.")
	} else {

		// Should create a track here.
		track := webrtc.NewVideoTrack()
		go webrtc.WriteToTrack(livestreamCursor, configuration, communication, mqttClient, track, codecs, decoder, decoderMutex)

		if config.Capture.ForwardWebRTC == "true" {
			// We get a request with an offer, but we'll forward it.
			for m := range communication.HandleLiveHDHandshake {
				// Forward SDP
				m.CloudKey = config.Key
				request, err := json.Marshal(m)
				if err == nil {
					mqttClient.Publish("kerberos/webrtc/request", 2, false, request)
				}
			}
		} else {
			log.Log.Info("HandleLiveStreamHD: Waiting for peer connections.")
			for handshake := range communication.HandleLiveHDHandshake {
				log.Log.Info("HandleLiveStreamHD: setting up a peer connection.")
				key := config.Key + "/" + handshake.Cuuid
				webrtc.CandidatesMutex.Lock()
				_, ok := webrtc.CandidateArrays[key]
				if !ok {
					webrtc.CandidateArrays[key] = make(chan string, 30)
				}
				webrtc.CandidatesMutex.Unlock()
				webrtc.InitializeWebRTCConnection(configuration, communication, mqttClient, track, handshake, webrtc.CandidateArrays[key])

			}
		}
	}
}

// VerifyHub godoc
// @Router /api/hub/verify [post]
// @ID verify-hub
// @Security Bearer
// @securityDefinitions.apikey Bearer
// @in header
// @name Authorization
// @Tags config
// @Param config body models.Config true "Config"
// @Summary Will verify the hub connectivity.
// @Description Will verify the hub connectivity.
// @Success 200 {object} models.APIResponse
func VerifyHub(c *gin.Context) {

	var config models.Config
	err := c.BindJSON(&config)

	if err == nil {
		hubKey := config.HubKey
		//hubPrivateKey := config.HubPrivateKey
		//hubSite := config.HubSite
		hubURI := config.HubURI

		content := []byte(`{"message": "fake-message"}`)
		body := bytes.NewReader(content)
		req, err := http.NewRequest("POST", hubURI+"/queue/test", body)
		if err == nil {
			req.Header.Set("X-Kerberos-Cloud-Key", hubKey)
			client := &http.Client{}

			resp, err := client.Do(req)
			if err == nil {
				body, err := ioutil.ReadAll(resp.Body)
				defer resp.Body.Close()
				if err == nil {
					if resp.StatusCode == 200 {
						c.JSON(200, body)
					} else {
						c.JSON(400, models.APIResponse{
							Data: "Something went wrong while reaching the Kerberos Hub API: " + string(body),
						})
					}
				} else {
					c.JSON(400, models.APIResponse{
						Data: "Something went wrong while ready the response body: " + err.Error(),
					})
				}
			} else {
				c.JSON(400, models.APIResponse{
					Data: "Something went wrong while reaching to the Kerberos Hub API: " + hubURI,
				})
			}
		} else {
			c.JSON(400, models.APIResponse{
				Data: "Something went wrong while creating the HTTP request: " + err.Error(),
			})
		}
	} else {
		c.JSON(400, models.APIResponse{
			Data: "Something went wrong while receiving the config " + err.Error(),
		})
	}
}

// VerifyPersistence godoc
// @Router /api/persistence/verify [post]
// @ID verify-persistence
// @Security Bearer
// @securityDefinitions.apikey Bearer
// @in header
// @name Authorization
// @Tags config
// @Param config body models.Config true "Config"
// @Summary Will verify the persistence.
// @Description Will verify the persistence.
// @Success 200 {object} models.APIResponse
func VerifyPersistence(c *gin.Context) {

	var config models.Config
	err := c.BindJSON(&config)
	if err != nil || config.Cloud != "" {

		if config.Cloud == "s3" {

			//fmt.Println("Uploading...")
			// timestamp_microseconds_instanceName_regionCoordinates_numberOfChanges_token
			// 1564859471_6-474162_oprit_577-283-727-375_1153_27.mp4
			// - Timestamp
			// - Size + - + microseconds
			// - device
			// - Region
			// - Number of changes
			// - Token

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
				c.JSON(400, models.APIResponse{
					Data: "Creation of Kerberos Hub connection failed: " + err.Error(),
				})
			} else {

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

				deviceKey := "fake-key"
				devicename := "justatest"
				coordinates := "200-200-400-400"
				eventToken := "769"

				timestamp := time.Now().Unix()
				fileName := strconv.FormatInt(timestamp, 10) + "_6-967003_justatest_200-200-400-400_24_769.mp4"
				content := []byte("test-file")
				body := bytes.NewReader(content)

				n, err := s3Client.PutObject(config.S3.Bucket,
					config.S3.Username+"/"+fileName,
					body,
					body.Size(),
					minio.PutObjectOptions{
						ContentType:  "video/mp4",
						StorageClass: "ONEZONE_IA",
						UserMetadata: map[string]string{
							"event-timestamp":         strconv.FormatInt(timestamp, 10),
							"event-microseconds":      deviceKey,
							"event-instancename":      devicename,
							"event-regioncoordinates": coordinates,
							"event-numberofchanges":   deviceKey,
							"event-token":             eventToken,
							"productid":               deviceKey,
							"publickey":               aws_access_key_id,
							"uploadtime":              "now",
						},
					})

				if err != nil {
					c.JSON(400, models.APIResponse{
						Data: "Upload of fake recording failed: " + err.Error(),
					})
				} else {
					c.JSON(200, models.APIResponse{
						Data: "Upload Finished: file has been uploaded to bucket: " + strconv.FormatInt(n, 10),
					})
				}
			}
		}

		if config.Cloud == "kstorage" {

			uri := config.KStorage.URI
			accessKey := config.KStorage.AccessKey
			secretAccessKey := config.KStorage.SecretAccessKey
			directory := config.KStorage.Directory
			provider := config.KStorage.Provider

			if err == nil && uri != "" && accessKey != "" && secretAccessKey != "" {
				var postData = []byte(`{"title":"Buy cheese and bread for breakfast."}`)
				client := &http.Client{}
				req, err := http.NewRequest("POST", uri+"/ping", bytes.NewReader(postData))

				req.Header.Add("X-Kerberos-Storage-AccessKey", accessKey)
				req.Header.Add("X-Kerberos-Storage-SecretAccessKey", secretAccessKey)
				resp, err := client.Do(req)

				if err == nil {
					body, err := ioutil.ReadAll(resp.Body)
					defer resp.Body.Close()
					if err == nil && resp.StatusCode == http.StatusOK {

						if provider != "" || directory != "" {

							hubKey := config.KStorage.CloudKey
							// This is the new way ;)
							if config.HubKey != "" {
								hubKey = config.HubKey
							}

							// Generate a random name.
							timestamp := time.Now().Unix()
							fileName := strconv.FormatInt(timestamp, 10) +
								"_6-967003_justatest_200-200-400-400_24_769.mp4"
							content := []byte("test-file")
							body := bytes.NewReader(content)
							//fileSize := int64(len(content))

							req, err := http.NewRequest("POST", uri+"/storage", body)
							if err == nil {

								req.Header.Set("Content-Type", "video/mp4")
								req.Header.Set("X-Kerberos-Storage-CloudKey", hubKey)
								req.Header.Set("X-Kerberos-Storage-AccessKey", accessKey)
								req.Header.Set("X-Kerberos-Storage-SecretAccessKey", secretAccessKey)
								req.Header.Set("X-Kerberos-Storage-Provider", provider)
								req.Header.Set("X-Kerberos-Storage-FileName", fileName)
								req.Header.Set("X-Kerberos-Storage-Device", "test")
								req.Header.Set("X-Kerberos-Storage-Capture", "IPCamera")
								req.Header.Set("X-Kerberos-Storage-Directory", directory)
								client := &http.Client{}

								resp, err := client.Do(req)

								if err == nil {
									if resp != nil {
										body, err := ioutil.ReadAll(resp.Body)
										defer resp.Body.Close()
										if err == nil {
											if resp.StatusCode == 200 {
												c.JSON(200, body)
											} else {
												c.JSON(400, models.APIResponse{
													Data: "Something went wrong while verifying your persistence settings. Make sure your provider is the same as the storage provider in your Kerberos Vault, and the relevant storage provider is configured properly.",
												})
											}
										}
									}
								} else {
									c.JSON(400, models.APIResponse{
										Data: "Upload of fake recording failed: " + err.Error(),
									})
								}
							} else {
								c.JSON(400, models.APIResponse{
									Data: "Something went wrong while creating /storage POST request." + err.Error(),
								})
							}
						} else {
							c.JSON(400, models.APIResponse{
								Data: "Provider and/or directory is missing from the request.",
							})
						}
					} else {
						c.JSON(400, models.APIResponse{
							Data: "Something went wrong while verifying storage credentials: " + string(body),
						})
					}
				} else {
					c.JSON(400, models.APIResponse{
						Data: "Something went wrong while verifying storage credentials:" + err.Error(),
					})
				}
			}
		}
	} else {
		c.JSON(400, models.APIResponse{
			Data: "No persistence was specified, so do not know what to verify:" + err.Error(),
		})
	}
}
