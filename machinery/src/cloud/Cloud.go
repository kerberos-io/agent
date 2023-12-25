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

	"github.com/elastic/go-sysinfo"
	"github.com/gin-gonic/gin"
	"github.com/golang-module/carbon/v2"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"net/http"
	"strconv"
	"time"

	"github.com/kerberos-io/agent/machinery/src/capture"
	"github.com/kerberos-io/agent/machinery/src/encryption"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/onvif"
	"github.com/kerberos-io/agent/machinery/src/packets"
	"github.com/kerberos-io/agent/machinery/src/utils"
	"github.com/kerberos-io/agent/machinery/src/webrtc"
)

func PendingUpload(configDirectory string) {
	ff, err := utils.ReadDirectory(configDirectory + "/data/cloud/")
	if err == nil {
		for _, f := range ff {
			log.Log.Info(f.Name())
		}
	}
}

func HandleUpload(configDirectory string, configuration *models.Configuration, communication *models.Communication) {

	log.Log.Debug("HandleUpload: started")

	config := configuration.Config
	watchDirectory := configDirectory + "/data/cloud/"

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
					if config.Cloud == "s3" || config.Cloud == "kerberoshub" {
						uploaded, configured, err = UploadKerberosHub(configuration, fileName)
					} else if config.Cloud == "kstorage" || config.Cloud == "kerberosvault" {
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
					} else if config.Cloud == "aws" {
						// Todo: need to be updated, was previously used for hub.
						uploaded, configured, err = UploadS3(configuration, fileName)
					} else if config.Cloud == "azure" {
						// Todo: implement azure upload
					} else if config.Cloud == "google" {
						// Todo: implement google upload
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
						if config.RemoveAfterUpload != "false" {
							err := os.Remove(configDirectory + "/data/recordings/" + fileName)
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

	var client *http.Client
	if os.Getenv("AGENT_TLS_INSECURE") == "true" {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client = &http.Client{Transport: tr}
	} else {
		client = &http.Client{}
	}

loop:
	for {

		config := configuration.Config

		if config.Offline == "true" {
			log.Log.Debug("HandleHeartBeat: stopping as Offline is enabled.")
		} else {

			hubURI := config.HeartbeatURI
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
				hubURI = config.HubURI + "/devices/heartbeat"
			}
			if config.HubKey != "" {
				key = config.HubKey
			}

			// Check if we have a friendly name or not.
			name := config.Name
			if config.FriendlyName != "" {
				name = config.FriendlyName
			}

			// Get some system information
			// like the uptime, hostname, memory usage, etc.
			system, _ := GetSystemInfo()

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
			if !communication.CameraConnected {
				cameraConnected = "false"
			}

			hasBackChannel := "false"
			if communication.HasBackChannel {
				hasBackChannel = "true"
			}

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
			onvifPresets := "false"
			var onvifPresetsList []byte
			var onvifEventsList []byte
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
						// Try to read out presets
						presets, err := onvif.GetPresetsFromDevice(device)
						if err == nil && len(presets) > 0 {
							onvifPresets = "true"
							onvifPresetsList, err = json.Marshal(presets)
							if err != nil {
								log.Log.Error("HandleHeartBeat: error while marshalling presets: " + err.Error())
								onvifPresetsList = []byte("[]")
							}
						} else {
							if err != nil {
								log.Log.Debug("HandleHeartBeat: error while getting presets: " + err.Error())
							} else {
								log.Log.Debug("HandleHeartBeat: no presets found.")
							}
							onvifPresetsList = []byte("[]")
						}
					} else {
						log.Log.Error("HandleHeartBeat: error while getting PTZ configurations: " + err.Error())
						onvifPresetsList = []byte("[]")
					}

					// We will also fetch some events, to know the status of the inputs and outputs.
					// More event types might be added.
					events, err := onvif.GetEventMessages(device)
					if err == nil && len(events) > 0 {
						onvifEventsList, err = json.Marshal(events)
						if err != nil {
							log.Log.Error("HandleHeartBeat: error while marshalling events: " + err.Error())
							onvifEventsList = []byte("[]")
						}
					} else {
						log.Log.Debug("HandleHeartBeat: no events found.")
						onvifEventsList = []byte("[]")
					}

				} else {
					log.Log.Error("HandleHeartBeat: error while connecting to ONVIF device: " + err.Error())
					onvifPresetsList = []byte("[]")
				}
			} else {
				log.Log.Debug("HandleHeartBeat: ONVIF is not enabled.")
				onvifPresetsList = []byte("[]")
			}

			// We need a hub URI and hub public key before we will send a heartbeat
			if hubURI != "" && key != "" {

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
						"onvif_presets": "%s",
						"onvif_presets_list": %s,
						"onvif_events_list": %s,
						"cameraConnected": "%s",
						"hasBackChannel": "%s",
						"numberoffiles" : "33",
						"timestamp" : 1564747908,
						"cameratype" : "IPCamera",
						"docker" : true,
						"kios" : false,
						"raspberrypi" : false
					}`, config.Key, system.Version, system.CPUId, username, key, name, isEnterprise, system.Hostname, system.Architecture, system.TotalMemory, system.UsedMemory, system.FreeMemory, system.ProcessUsedMemory, macs, ips, "0", "0", "0", uptimeString, boottimeString, config.HubSite, onvifEnabled, onvifZoom, onvifPanTilt, onvifPresets, onvifPresetsList, onvifEventsList, cameraConnected, hasBackChannel)

				// Get the private key to encrypt the data using symmetric encryption: AES.
				privateKey := config.HubPrivateKey
				if privateKey != "" {
					// Encrypt the data using AES.
					encrypted, err := encryption.AesEncrypt([]byte(object), privateKey)
					if err != nil {
						encrypted = []byte("")
						log.Log.Error("HandleHeartBeat: error while encrypting data: " + err.Error())
					}

					// Base64 encode the encrypted data.
					encryptedBase64 := base64.StdEncoding.EncodeToString(encrypted)
					object = fmt.Sprintf(`{
						"cloudpublicKey": "%s",
						"encrypted" : %t,
						"encryptedData" : "%s"
					}`, config.HubKey, true, encryptedBase64)
				}

				var jsonStr = []byte(object)
				buffy := bytes.NewBuffer(jsonStr)
				req, _ := http.NewRequest("POST", hubURI, buffy)
				req.Header.Set("Content-Type", "application/json")
				resp, err := client.Do(req)
				if resp != nil {
					resp.Body.Close()
				}
				if err == nil && resp.StatusCode == 200 {
					communication.CloudTimestamp.Store(time.Now().Unix())
					log.Log.Info("HandleHeartBeat: (200) Heartbeat received by Kerberos Hub.")
				} else {
					if communication.CloudTimestamp != nil && communication.CloudTimestamp.Load() != nil {
						communication.CloudTimestamp.Store(int64(0))
					}
					log.Log.Error("HandleHeartBeat: (400) Something went wrong while sending to Kerberos Hub.")
				}
			} else {
				log.Log.Error("HandleHeartBeat: Disabled as we do not have a public key defined.")
			}

			// If we have a Kerberos Vault connected, we will also send some analytics
			// to that service.
			vaultURI = config.KStorage.URI
			if vaultURI != "" {

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
					"onvif_presets": "%s",
					"onvif_presets_list": %s,
					"cameraConnected": "%s",
					"numberoffiles" : "33",
					"timestamp" : 1564747908,
					"cameratype" : "IPCamera",
					"docker" : true,
					"kios" : false,
					"raspberrypi" : false
				}`, config.Key, system.Version, system.CPUId, username, key, name, isEnterprise, system.Hostname, system.Architecture, system.TotalMemory, system.UsedMemory, system.FreeMemory, system.ProcessUsedMemory, macs, ips, "0", "0", "0", uptimeString, boottimeString, config.HubSite, onvifEnabled, onvifZoom, onvifPanTilt, onvifPresets, onvifPresetsList, cameraConnected)

				var jsonStr = []byte(object)
				buffy := bytes.NewBuffer(jsonStr)
				req, _ := http.NewRequest("POST", vaultURI+"/devices/heartbeat", buffy)
				req.Header.Set("Content-Type", "application/json")

				resp, err := client.Do(req)
				if resp != nil {
					resp.Body.Close()
				}
				if err == nil && resp.StatusCode == 200 {
					log.Log.Info("HandleHeartBeat: (200) Heartbeat received by Kerberos Vault.")
				} else {
					log.Log.Error("HandleHeartBeat: (400) Something went wrong while sending to Kerberos Vault.")
				}
			}
		}

		// This will check if we need to stop the thread,
		// because of a reconfiguration.
		select {
		case <-communication.HandleHeartBeat:
			break loop
		case <-time.After(10 * time.Second):
		}
	}

	log.Log.Debug("HandleHeartBeat: finished")
}

func HandleLiveStreamSD(livestreamCursor *packets.QueueCursor, configuration *models.Configuration, communication *models.Communication, mqttClient mqtt.Client, rtspClient capture.RTSPClient) {

	log.Log.Debug("cloud.HandleLiveStreamSD(): started")

	config := configuration.Config

	// If offline made is enabled, we will stop the thread.
	if config.Offline == "true" {
		log.Log.Debug("cloud.HandleLiveStreamSD(): stopping as Offline is enabled.")
	} else {

		// Check if we need to enable the live stream
		if config.Capture.Liveview != "false" {

			hubKey := ""
			if config.Cloud == "s3" && config.S3 != nil && config.S3.Publickey != "" {
				hubKey = config.S3.Publickey
			} else if config.Cloud == "kstorage" && config.KStorage != nil && config.KStorage.CloudKey != "" {
				hubKey = config.KStorage.CloudKey
			}
			// This is the new way ;)
			if config.HubKey != "" {
				hubKey = config.HubKey
			}

			lastLivestreamRequest := int64(0)

			var cursorError error
			var pkt packets.Packet

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
				log.Log.Info("cloud.HandleLiveStreamSD(): Sending base64 encoded images to MQTT.")
				img, err := rtspClient.DecodePacket(pkt)
				if err == nil {
					bytes, _ := utils.ImageToBytes(&img)
					encoded := base64.StdEncoding.EncodeToString(bytes)

					valueMap := make(map[string]interface{})
					valueMap["image"] = encoded
					message := models.Message{
						Payload: models.Payload{
							Action:   "receive-sd-stream",
							DeviceId: configuration.Config.Key,
							Value:    valueMap,
						},
					}
					payload, err := models.PackageMQTTMessage(configuration, message)
					if err == nil {
						mqttClient.Publish("kerberos/hub/"+hubKey, 0, false, payload)
					} else {
						log.Log.Info("cloud.HandleLiveStreamSD(): something went wrong while sending acknowledge config to hub: " + string(payload))
					}
				}
			}

		} else {
			log.Log.Debug("cloud.HandleLiveStreamSD(): stopping as Liveview is disabled.")
		}
	}

	log.Log.Debug("cloud.HandleLiveStreamSD(): finished")
}

func HandleLiveStreamHD(livestreamCursor *packets.QueueCursor, configuration *models.Configuration, communication *models.Communication, mqttClient mqtt.Client, rtspClient capture.RTSPClient) {

	config := configuration.Config

	if config.Offline == "true" {
		log.Log.Debug("HandleLiveStreamHD: stopping as Offline is enabled.")
	} else {

		// Check if we need to enable the live stream
		if config.Capture.Liveview != "false" {

			// Should create a track here.
			streams, _ := rtspClient.GetStreams()
			videoTrack := webrtc.NewVideoTrack(streams)
			audioTrack := webrtc.NewAudioTrack(streams)
			go webrtc.WriteToTrack(livestreamCursor, configuration, communication, mqttClient, videoTrack, audioTrack, rtspClient)

			if config.Capture.ForwardWebRTC == "true" {

			} else {
				log.Log.Info("HandleLiveStreamHD: Waiting for peer connections.")
				for handshake := range communication.HandleLiveHDHandshake {
					log.Log.Info("HandleLiveStreamHD: setting up a peer connection.")
					go webrtc.InitializeWebRTCConnection(configuration, communication, mqttClient, videoTrack, audioTrack, handshake)
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
// @Tags general
// @Param config body models.Config true "Config"
// @Summary Will verify the hub connectivity.
// @Description Will verify the hub connectivity.
// @Success 200 {object} models.APIResponse
func VerifyHub(c *gin.Context) {

	var config models.Config
	err := c.BindJSON(&config)

	if err == nil {
		hubURI := config.HubURI
		publicKey := config.HubKey
		privateKey := config.HubPrivateKey

		req, err := http.NewRequest("POST", hubURI+"/subscription/verify", nil)
		if err == nil {
			req.Header.Set("X-Kerberos-Hub-PublicKey", publicKey)
			req.Header.Set("X-Kerberos-Hub-PrivateKey", privateKey)
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
// @Tags general
// @Param config body models.Config true "Config"
// @Summary Will verify the persistence.
// @Description Will verify the persistence.
// @Success 200 {object} models.APIResponse
func VerifyPersistence(c *gin.Context, configDirectory string) {

	var config models.Config
	err := c.BindJSON(&config)
	if err != nil || config.Cloud != "" {

		if config.Cloud == "dropbox" {
			VerifyDropbox(config, c)
		} else if config.Cloud == "s3" || config.Cloud == "kerberoshub" {

			if config.HubURI == "" ||
				config.HubKey == "" ||
				config.HubPrivateKey == "" ||
				config.S3.Region == "" {
				msg := "VerifyPersistence: Kerberos Hub not properly configured."
				log.Log.Info(msg)
				c.JSON(400, models.APIResponse{
					Data: msg,
				})
			} else {

				// Open test-480p.mp4
				file, err := os.Open(configDirectory + "/data/test-480p.mp4")
				if err != nil {
					msg := "VerifyPersistence: error reading test-480p.mp4: " + err.Error()
					log.Log.Error(msg)
					c.JSON(400, models.APIResponse{
						Data: msg,
					})
				}
				defer file.Close()

				req, err := http.NewRequest("POST", config.HubURI+"/storage/upload", file)
				if err != nil {
					msg := "VerifyPersistence: error reading Kerberos Hub HEAD request, " + config.HubURI + "/storage: " + err.Error()
					log.Log.Error(msg)
					c.JSON(400, models.APIResponse{
						Data: msg,
					})
				}

				timestamp := time.Now().Unix()
				fileName := strconv.FormatInt(timestamp, 10) +
					"_6-967003_" + config.Name + "_200-200-400-400_24_769.mp4"
				req.Header.Set("X-Kerberos-Storage-FileName", fileName)
				req.Header.Set("X-Kerberos-Storage-Capture", "IPCamera")
				req.Header.Set("X-Kerberos-Storage-Device", config.Key)
				req.Header.Set("X-Kerberos-Hub-PublicKey", config.HubKey)
				req.Header.Set("X-Kerberos-Hub-PrivateKey", config.HubPrivateKey)
				req.Header.Set("X-Kerberos-Hub-Region", config.S3.Region)

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

				if err == nil && resp != nil {
					if resp.StatusCode == 200 {
						msg := "VerifyPersistence: Upload allowed using the credentials provided (" + config.HubKey + ", " + config.HubPrivateKey + ")"
						log.Log.Info(msg)
						c.JSON(200, models.APIResponse{
							Data: msg,
						})
					} else {
						msg := "VerifyPersistence: Upload NOT allowed using the credentials provided (" + config.HubKey + ", " + config.HubPrivateKey + ")"
						log.Log.Info(msg)
						c.JSON(400, models.APIResponse{
							Data: msg,
						})
					}
				} else {
					msg := "VerifyPersistence: Error creating Kerberos Hub request"
					log.Log.Info(msg)
					c.JSON(400, models.APIResponse{
						Data: msg,
					})
				}
			}

		} else if config.Cloud == "kstorage" || config.Cloud == "kerberosvault" {

			uri := config.KStorage.URI
			accessKey := config.KStorage.AccessKey
			secretAccessKey := config.KStorage.SecretAccessKey
			directory := config.KStorage.Directory
			provider := config.KStorage.Provider

			if err == nil && uri != "" && accessKey != "" && secretAccessKey != "" {

				var client *http.Client
				if os.Getenv("AGENT_TLS_INSECURE") == "true" {
					tr := &http.Transport{
						TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
					}
					client = &http.Client{Transport: tr}
				} else {
					client = &http.Client{}
				}

				req, err := http.NewRequest("POST", uri+"/ping", nil)
				req.Header.Add("X-Kerberos-Storage-AccessKey", accessKey)
				req.Header.Add("X-Kerberos-Storage-SecretAccessKey", secretAccessKey)
				resp, err := client.Do(req)

				if err == nil {
					body, err := ioutil.ReadAll(resp.Body)
					defer resp.Body.Close()
					if err == nil && resp.StatusCode == http.StatusOK {

						if provider != "" || directory != "" {

							// Generate a random name.
							timestamp := time.Now().Unix()
							fileName := strconv.FormatInt(timestamp, 10) +
								"_6-967003_" + config.Name + "_200-200-400-400_24_769.mp4"

							// Open test-480p.mp4
							file, err := os.Open(configDirectory + "/data/test-480p.mp4")
							if err != nil {
								msg := "VerifyPersistence: error reading test-480p.mp4: " + err.Error()
								log.Log.Error(msg)
								c.JSON(400, models.APIResponse{
									Data: msg,
								})
							}
							defer file.Close()

							req, err := http.NewRequest("POST", uri+"/storage", file)
							if err == nil {

								req.Header.Set("Content-Type", "video/mp4")
								req.Header.Set("X-Kerberos-Storage-CloudKey", config.HubKey)
								req.Header.Set("X-Kerberos-Storage-AccessKey", accessKey)
								req.Header.Set("X-Kerberos-Storage-SecretAccessKey", secretAccessKey)
								req.Header.Set("X-Kerberos-Storage-Provider", provider)
								req.Header.Set("X-Kerberos-Storage-FileName", fileName)
								req.Header.Set("X-Kerberos-Storage-Device", config.Key)
								req.Header.Set("X-Kerberos-Storage-Capture", "IPCamera")
								req.Header.Set("X-Kerberos-Storage-Directory", directory)

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

								if err == nil {
									if resp != nil {
										body, err := ioutil.ReadAll(resp.Body)
										defer resp.Body.Close()
										if err == nil {
											if resp.StatusCode == 200 {
												c.JSON(200, body)
											} else {
												c.JSON(400, models.APIResponse{
													Data: "VerifyPersistence: Something went wrong while verifying your persistence settings. Make sure your provider is the same as the storage provider in your Kerberos Vault, and the relevant storage provider is configured properly.",
												})
											}
										}
									}
								} else {
									c.JSON(400, models.APIResponse{
										Data: "VerifyPersistence: Upload of fake recording failed: " + err.Error(),
									})
								}
							} else {
								c.JSON(400, models.APIResponse{
									Data: "VerifyPersistence: Something went wrong while creating /storage POST request." + err.Error(),
								})
							}
						} else {
							c.JSON(400, models.APIResponse{
								Data: "VerifyPersistence: Provider and/or directory is missing from the request.",
							})
						}
					} else {
						c.JSON(400, models.APIResponse{
							Data: "VerifyPersistence: Something went wrong while verifying storage credentials: " + string(body),
						})
					}
				} else {
					c.JSON(400, models.APIResponse{
						Data: "VerifyPersistence: Something went wrong while verifying storage credentials:" + err.Error(),
					})
				}
			} else {
				c.JSON(400, models.APIResponse{
					Data: "VerifyPersistence: please fill-in the required Kerberos Vault credentials.",
				})
			}
		}
	} else {
		c.JSON(400, models.APIResponse{
			Data: "VerifyPersistence: No persistence was specified, so do not know what to verify:" + err.Error(),
		})
	}
}
