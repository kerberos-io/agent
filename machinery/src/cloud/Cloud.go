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

	"github.com/elastic/go-sysinfo"
	"github.com/gin-gonic/gin"
	"github.com/golang-module/carbon/v2"
	"github.com/kerberos-io/joy4/av/pubsub"
	"github.com/minio/minio-go/v6"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	av "github.com/kerberos-io/joy4/av"
	"github.com/kerberos-io/joy4/cgo/ffmpeg"

	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/kerberos-io/agent/machinery/src/computervision"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/onvif"
	"github.com/kerberos-io/agent/machinery/src/utils"
	"github.com/kerberos-io/agent/machinery/src/webrtc"
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

		// Half a second delay between two uploads
		delay := 500 * time.Millisecond

	loop:
		for {
			// This will check if we need to stop the thread,
			// because of a reconfiguration.
			select {
			case <-communication.HandleUpload:
				break loop
			case <-time.After(2 * time.Second):
			}

			ff, err := utils.ReadDirectory(watchDirectory)
			if err != nil {
				log.Log.Error("HandleUpload: " + err.Error())
			} else {
				for _, f := range ff {

					// This will check if we need to stop the thread,
					// because of a reconfiguration.
					select {
					case <-communication.HandleUpload:
						break loop
					default:
					}

					fileName := f.Name()
					uploaded := false
					configured := false
					err = nil
					if config.Cloud == "s3" {
						uploaded, configured, err = UploadS3(configuration, fileName)
					} else if config.Cloud == "kstorage" {
						uploaded, configured, err = UploadKerberosVault(configuration, fileName)
					} else if config.Cloud == "dropbox" {
						uploaded, configured, err = UploadDropbox(configuration, fileName)
					} else if config.Cloud == "gdrive" {
						// Todo: implement gdrive upload
					} else if config.Cloud == "onedrive" {
						// Todo: implement onedrive upload
					} else if config.Cloud == "minio" {
						// Todo: implement minio upload
					} else if config.Cloud == "webdav" {
						// Todo: implement webdav upload
					} else if config.Cloud == "ftp" {
						// Todo: implement ftp upload
					} else if config.Cloud == "sftp" {
						// Todo: implement sftp upload
					}
					// And so on... (have a look here -> https://github.com/kerberos-io/agent/issues/95)

					// Check if the file is uploaded, if so, remove it.
					if uploaded {
						delay = 500 * time.Millisecond // reset
						err := os.Remove(watchDirectory + fileName)
						if err != nil {
							log.Log.Error("HandleUpload: " + err.Error())
						}

						// Check if we need to remove the original recording
						// removeAfterUpload is set to false by default
						if config.RemoveAfterUpload == "true" {
							err := os.Remove("./data/recordings/" + fileName)
							if err != nil {
								log.Log.Error("HandleUpload: " + err.Error())
							}
						}
					} else if !configured {
						err := os.Remove(watchDirectory + fileName)
						if err != nil {
							log.Log.Error("HandleUpload: " + err.Error())
						}
					} else {
						delay = 20 * time.Second // slow down
						if err != nil {
							log.Log.Error("HandleUpload: " + err.Error())
						}
					}

					time.Sleep(delay)
				}
			}
		}
	}

	log.Log.Debug("HandleUpload: finished")
}

func GetSystemInfo() (models.System, error) {
	var usedMem uint64 = 0
	var totalMem uint64 = 0
	var freeMem uint64 = 0

	var processUsedMem uint64 = 0

	architecture := ""
	cpuId := ""
	KernelVersion := ""
	agentVersion := ""
	var MACs []string
	var IPs []string
	hostname := ""
	bootTime := time.Time{}

	// Read agent version
	version, err := os.Open("./version")
	defer version.Close()
	agentVersion = "unknown"
	if err == nil {
		agentVersionBytes, err := ioutil.ReadAll(version)
		agentVersion = string(agentVersionBytes)
		if err != nil {
			log.Log.Error(err.Error())
		}
	}

	host, err := sysinfo.Host()
	if err == nil {
		cpuId = host.Info().UniqueID
		architecture = host.Info().Architecture
		KernelVersion = host.Info().KernelVersion
		MACs = host.Info().MACs
		IPs = host.Info().IPs
		hostname = host.Info().Hostname
		bootTime = host.Info().BootTime
		memory, err := host.Memory()
		if err == nil {
			usedMem = memory.Used
			totalMem = memory.Total
			freeMem = memory.Free
		}
	}

	process, err := sysinfo.Self()
	if err == nil {
		memInfo, err := process.Memory()
		if err == nil {
			processUsedMem = memInfo.Resident
		}
	}

	system := models.System{
		Hostname:          hostname,
		CPUId:             cpuId,
		KernelVersion:     KernelVersion,
		Version:           agentVersion,
		MACs:              MACs,
		IPs:               IPs,
		BootTime:          uint64(bootTime.Unix()),
		Architecture:      architecture,
		UsedMemory:        usedMem,
		TotalMemory:       totalMem,
		FreeMemory:        freeMem,
		ProcessUsedMemory: processUsedMem,
	}

	return system, nil
}

func HandleHeartBeat(configuration *models.Configuration, communication *models.Communication, uptimeStart time.Time) {
	log.Log.Debug("HandleHeartBeat: started")

loop:
	for {

		config := configuration.Config

		if config.Offline == "true" {
			log.Log.Debug("HandleHeartBeat: stopping as Offline is enabled.")
		} else {

			url := config.HeartbeatURI
			key := ""
			username := ""
			vaultURI := ""

			username = config.S3.Username
			if config.Cloud == "s3" && config.S3 != nil && config.S3.Publickey != "" {
				username = config.S3.Username
				key = config.S3.Publickey
			} else if config.Cloud == "kstorage" && config.KStorage != nil && config.KStorage.CloudKey != "" {
				key = config.KStorage.CloudKey
				username = config.KStorage.Directory
			}

			// This is the new way ;)
			if config.HubURI != "" {
				url = config.HubURI + "/devices/heartbeat"
			}
			if config.HubKey != "" {
				key = config.HubKey
			}

			if key != "" {
				// Check if we have a friendly name or not.
				name := config.Name
				if config.FriendlyName != "" {
					name = config.FriendlyName
				}

				// Get some system information
				// like the uptime, hostname, memory usage, etc.
				system, _ := GetSystemInfo()

				// We will formated the uptime to a human readable format
				// this will be used on Kerberos Hub: Uptime -> 1 day and 2 hours.
				uptimeFormatted := uptimeStart.Format("2006-01-02 15:04:05")
				uptimeString := carbon.Parse(uptimeFormatted).DiffForHumans()
				uptimeString = strings.ReplaceAll(uptimeString, "ago", "")

				// Do the same for boottime
				bootTimeFormatted := time.Unix(int64(system.BootTime), 0).Format("2006-01-02 15:04:05")
				boottimeString := carbon.Parse(bootTimeFormatted).DiffForHumans()
				boottimeString = strings.ReplaceAll(boottimeString, "ago", "")

				// We'll check which mode is enabled for the camera.
				onvifEnabled := "false"
				onvifZoom := "false"
				onvifPanTilt := "false"
				onvifVersion := "unknown"

				if config.Capture.IPCamera.ONVIFXAddr != "" {
					cameraConfiguration := configuration.Config.Capture.IPCamera
					device, err := onvif.ConnectToOnvifDevice(&cameraConfiguration)
					if err == nil {
						configurations, err := onvif.GetPTZConfigurationsFromDevice(device)
						if err == nil {
							onvifEnabled = "true"
							_, canZoom, canPanTilt := onvif.GetPTZFunctionsFromDevice(configurations)
							if canZoom {
								onvifZoom = "true"
							}
							if canPanTilt {
								onvifPanTilt = "true"
							}
						}
						// Get the ONVIF version from the device.
						onvifVersion, err = onvif.GetONVIFVersionFromDevice(device)
					}
				}

				// Check if the agent is running inside a cluster (Kerberos Factory) or as
				// an open source agent
				isEnterprise := false
				if os.Getenv("DEPLOYMENT") == "factory" || os.Getenv("MACHINERY_ENVIRONMENT") == "kubernetes" {
					isEnterprise = true
				}

				// Congert to string
				macs, _ := json.Marshal(system.MACs)
				ips, _ := json.Marshal(system.IPs)
				cameraConnected := "true"
				if communication.CameraConnected == false {
					cameraConnected = "false"
				}

				var object = fmt.Sprintf(`{
						"key" : "%s",
						"version" : "3.0.0",
						"release" : "%s",
						"cpuid" : "%s",
						"clouduser" : "%s",
						"cloudpublickey" : "%s",
						"cameraname" : "%s",
						"enterprise" : %t,
						"hostname" : "%s",
						"architecture" : "%s",
						"totalMemory" : "%d",
						"usedMemory" : "%d",
						"freeMemory" : "%d",
						"processMemory" : "%d",
						"mac_list" : %s,
						"ip_list" : %s,
						"board" : "",
						"disk1size" : "%s",
						"disk3size" : "%s",
						"diskvdasize" :  "%s",
						"uptime" : "%s",
						"boot_time" : "%s",
						"siteID" : "%s",
						"onvif" : "%s",
						"onvif_zoom" : "%s",
						"onvif_pantilt" : "%s",
						"onvif_version" : "%s",
						"cameraConnected": "%s",
						"numberoffiles" : "33",
						"timestamp" : 1564747908,
						"cameratype" : "IPCamera",
						"docker" : true,
						"kios" : false,
						"raspberrypi" : false
					}`, config.Key, system.Version, system.CPUId, username, key, name, isEnterprise, system.Hostname, system.Architecture, system.TotalMemory, system.UsedMemory, system.FreeMemory, system.ProcessUsedMemory, macs, ips, "0", "0", "0", uptimeString, boottimeString, config.HubSite, onvifEnabled, onvifZoom, onvifPanTilt, onvifVersion, cameraConnected)

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
					communication.CloudTimestamp.Store(0)
					log.Log.Error("HandleHeartBeat: (400) Something went wrong while sending to Kerberos Hub.")
				}

				// If we have a Kerberos Vault connected, we will also send some analytics
				// to that service.
				vaultURI = config.KStorage.URI
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
			} else {
				log.Log.Error("HandleHeartBeat: Disabled as we do not have a public key defined.")
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

	log.Log.Debug("HandleHeartBeat: finished")
}

func HandleLiveStreamSD(livestreamCursor *pubsub.QueueCursor, configuration *models.Configuration, communication *models.Communication, mqttClient mqtt.Client, decoder *ffmpeg.VideoDecoder, decoderMutex *sync.Mutex) {

	log.Log.Debug("HandleLiveStreamSD: started")

	config := configuration.Config

	// If offline made is enabled, we will stop the thread.
	if config.Offline == "true" {
		log.Log.Debug("HandleLiveStreamSD: stopping as Offline is enabled.")
	} else {

		// Check if we need to enable the live stream
		if config.Capture.Liveview != "false" {

			// Allocate frame
			frame := ffmpeg.AllocVideoFrame()

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
				sendImage(frame, topic, mqttClient, pkt, decoder, decoderMutex)
			}

			// Cleanup the frame.
			frame.Free()

		} else {
			log.Log.Debug("HandleLiveStreamSD: stopping as Liveview is disabled.")
		}
	}

	log.Log.Debug("HandleLiveStreamSD: finished")
}

func sendImage(frame *ffmpeg.VideoFrame, topic string, mqttClient mqtt.Client, pkt av.Packet, decoder *ffmpeg.VideoDecoder, decoderMutex *sync.Mutex) {
	_, err := computervision.GetRawImage(frame, pkt, decoder, decoderMutex)
	if err == nil {
		bytes, _ := computervision.ImageToBytes(&frame.Image)
		encoded := base64.StdEncoding.EncodeToString(bytes)
		mqttClient.Publish(topic, 0, false, encoded)
	}
}

func HandleLiveStreamHD(livestreamCursor *pubsub.QueueCursor, configuration *models.Configuration, communication *models.Communication, mqttClient mqtt.Client, codecs []av.CodecData, decoder *ffmpeg.VideoDecoder, decoderMutex *sync.Mutex) {

	config := configuration.Config

	if config.Offline == "true" {
		log.Log.Debug("HandleLiveStreamHD: stopping as Offline is enabled.")
	} else {

		// Check if we need to enable the live stream
		if config.Capture.Liveview != "false" {

			// Should create a track here.
			videoTrack := webrtc.NewVideoTrack(codecs)
			audioTrack := webrtc.NewAudioTrack(codecs)
			go webrtc.WriteToTrack(livestreamCursor, configuration, communication, mqttClient, videoTrack, audioTrack, codecs, decoder, decoderMutex)

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
					webrtc.InitializeWebRTCConnection(configuration, communication, mqttClient, videoTrack, audioTrack, handshake, webrtc.CandidateArrays[key])

				}
			}

		} else {
			log.Log.Debug("HandleLiveStreamHD: stopping as Liveview is disabled.")
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

		if config.Cloud == "dropbox" {
			VerifyDropbox(config, c)
		} else if config.Cloud == "s3" {

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
		} else if config.Cloud == "kstorage" {

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
