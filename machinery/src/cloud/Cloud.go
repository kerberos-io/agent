package cloud

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dromara/carbon/v2"
	"github.com/elastic/go-sysinfo"
	"github.com/gin-gonic/gin"

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
						delay = 5 * time.Second // slow down
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
	agentVersion = "unknown"
	if err == nil {
		defer version.Close()
		agentVersionBytes, err := io.ReadAll(version)
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
	log.Log.Debug("cloud.HandleHeartBeat(): started")

	var client *http.Client
	if os.Getenv("AGENT_TLS_INSECURE") == "true" {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client = &http.Client{Transport: tr}
	} else {
		client = &http.Client{}
	}

	kerberosAgentVersion := utils.VERSION

	// Create a loop pull point address, which we will use to retrieve async events
	// As you'll read below camera manufactures are having different implementations of events.
	var pullPointAddressLoopState string
	if configuration.Config.Capture.IPCamera.ONVIFXAddr != "" {
		cameraConfiguration := configuration.Config.Capture.IPCamera
		device, _, err := onvif.ConnectToOnvifDevice(&cameraConfiguration)
		if err != nil {
			pullPointAddressLoopState, err = onvif.CreatePullPointSubscription(device)
			if err != nil {
				log.Log.Error("cloud.HandleHeartBeat(): error while creating pull point subscription: " + err.Error())
			}
		}
	}

loop:
	for {
		// Configuration migh have changed, so we will reload it.
		config := configuration.Config

		// We'll check ONVIF capabilitites anyhow.. Verify if we have PTZ, presets and inputs/outputs.
		// For the inputs we will keep track of a the inputs and outputs state.
		onvifEnabled := "false"
		onvifZoom := "false"
		onvifPanTilt := "false"
		onvifPresets := "false"
		var onvifPresetsList []byte
		var onvifEventsList []byte
		if config.Capture.IPCamera.ONVIFXAddr != "" {
			cameraConfiguration := configuration.Config.Capture.IPCamera
			device, _, err := onvif.ConnectToOnvifDevice(&cameraConfiguration)
			if err == nil {
				// We will try to retrieve the PTZ configurations from the device.
				onvifEnabled = "true"
				configurations, err := onvif.GetPTZConfigurationsFromDevice(device)
				if err == nil {
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
							log.Log.Error("cloud.HandleHeartBeat(): error while marshalling presets: " + err.Error())
							onvifPresetsList = []byte("[]")
						}
					} else {
						if err != nil {
							log.Log.Debug("cloud.HandleHeartBeat(): error while getting presets: " + err.Error())
						} else {
							log.Log.Debug("cloud.HandleHeartBeat(): no presets found.")
						}
						onvifPresetsList = []byte("[]")
					}
				} else {
					log.Log.Debug("cloud.HandleHeartBeat(): error while getting PTZ configurations: " + err.Error())
					onvifPresetsList = []byte("[]")
				}

				// We will also fetch some events, to know the status of the inputs and outputs.
				// More event types might be added.
				// -- We have two differen pull point subscriptions, one for the initials events and one for the loop.
				// -- Some cameras do send recurrent events, others don't.
				//   a. For some older Hikvision models, events are send repeatedly (if input is high) with the strong state (set to false).
				//      - In this scenarion we are using a polling mechanism and set a timestamp to understand if the input is still active.
				//   b. For some newer Hikvision models, Avigilon, events are send only once (if state is set active).
				//      - In this scenario we are creating a new subscription to retrieve the initial (current) state of the inputs and outputs.

				// Get a new pull point address, to get the initiatal state of the inputs and outputs.
				pullPointAddressInitialState, err := onvif.CreatePullPointSubscription(device)
				if err != nil {
					log.Log.Error("cloud.HandleHeartBeat(): error while creating pull point subscription: " + err.Error())
				}
				if pullPointAddressInitialState != "" {
					log.Log.Debug("cloud.HandleHeartBeat(): Fetching events from pullPointAddressInitialState")
					events, err := onvif.GetEventMessages(device, pullPointAddressInitialState)
					log.Log.Debug("cloud.HandleHeartBeat(): Completed fetching events from pullPointAddressInitialState")
					if err == nil && len(events) > 0 {
						onvifEventsList, err = json.Marshal(events)
						if err != nil {
							log.Log.Error("cloud.HandleHeartBeat(): error while marshalling events: " + err.Error())
							onvifEventsList = []byte("[]")
						}
					} else if err != nil {
						log.Log.Error("cloud.HandleHeartBeat(): error while getting events: " + err.Error())
						onvifEventsList = []byte("[]")
					} else if len(events) == 0 {
						log.Log.Debug("cloud.HandleHeartBeat(): no events found.")
						onvifEventsList = []byte("[]")
					}
					onvif.UnsubscribePullPoint(device, pullPointAddressInitialState)
				}

				// We do a second run an a long-living subscription to get the events asynchronously.
				if pullPointAddressLoopState != "" {
					log.Log.Debug("cloud.HandleHeartBeat(): Fetching events from pullPointAddressLoopState")
					events, err := onvif.GetEventMessages(device, pullPointAddressLoopState)
					log.Log.Debug("cloud.HandleHeartBeat(): Completed fetching events from pullPointAddressLoopState")
					if err == nil && len(events) > 0 {
						onvifEventsList, err = json.Marshal(events)
						if err != nil {
							log.Log.Error("cloud.HandleHeartBeat(): error while marshalling events: " + err.Error())
							onvifEventsList = []byte("[]")
						}
					} else if err != nil {
						log.Log.Error("cloud.HandleHeartBeat(): error while getting events: " + err.Error())
						onvifEventsList = []byte("[]")
						pullPointAddressLoopState, err = onvif.CreatePullPointSubscription(device)
						if err != nil {
							log.Log.Error("cloud.HandleHeartBeat(): error while creating pull point subscription: " + err.Error())
						}
					} else if len(events) == 0 {
						log.Log.Debug("cloud.HandleHeartBeat(): no events found.")
						onvifEventsList = []byte("[]")
					}
				} else {
					log.Log.Debug("cloud.HandleHeartBeat(): no pull point address found.")
					pullPointAddressLoopState, err = onvif.CreatePullPointSubscription(device)
					if err != nil {
						log.Log.Error("cloud.HandleHeartBeat(): error while creating pull point subscription: " + err.Error())
					}
				}

				// It also might be that events are not supported by the camera, in that case we will try to get the digital inputs and outputs.
				// Through the `device` API, the `GetDigitalInputs` and `GetDigitalOutputs` functions are called.
				// The disadvantage of this approach is that we don't have the state of the inputs and outputs (which is crazy..)

				if pullPointAddressInitialState == "" && pullPointAddressLoopState == "" {
					var events []onvif.ONVIFEvents
					outputs, err := onvif.GetRelayOutputs(device)
					if err != nil {
						log.Log.Debug("cloud.HandleHeartBeat(): error while getting relay outputs: " + err.Error())
					} else {
						for _, output := range outputs.RelayOutputs {
							event := onvif.ONVIFEvents{
								Key:       string(output.Token),
								Value:     "false",
								Type:      "output",
								Timestamp: time.Now().Unix(),
							}
							events = append(events, event)
						}
					}

					inputs, err := onvif.GetDigitalInputs(device)
					if err != nil {
						log.Log.Debug("cloud.HandleHeartBeat(): error while getting digital inputs: " + err.Error())
					} else {
						for _, input := range inputs.DigitalInputs {
							event := onvif.ONVIFEvents{
								Key:       string(input.Token),
								Value:     "false",
								Type:      "input",
								Timestamp: time.Now().Unix(),
							}
							events = append(events, event)
						}
					}

					// Marshal the events
					onvifEventsList, err = json.Marshal(events)
					if err != nil {
						log.Log.Error("cloud.HandleHeartBeat(): error while marshalling events: " + err.Error())
						onvifEventsList = []byte("[]")
					}
				}
			} else {
				log.Log.Error("cloud.HandleHeartBeat(): error while connecting to ONVIF device: " + err.Error())
				onvifPresetsList = []byte("[]")
				onvifEventsList = []byte("[]")
			}
		} else {
			log.Log.Debug("cloud.HandleHeartBeat(): ONVIF is not enabled.")
			onvifPresetsList = []byte("[]")
			onvifEventsList = []byte("[]")
		}

		// We'll capture some more metrics, and send it to Hub, if not in offline mode ofcourse ;) ;)
		if config.Offline == "true" {
			log.Log.Debug("cloud.HandleHeartBeat(): stopping as Offline is enabled.")
		} else {

			hubURI := config.HeartbeatURI
			key := ""
			username := ""
			vaultURI := ""

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

			hub_encryption := "false"
			if config.HubEncryption == "true" {
				hub_encryption = "true"
			}

			e2e_encryption := "false"
			if config.Encryption != nil && config.Encryption.Enabled == "true" {
				e2e_encryption = "true"
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

			// We need a hub URI and hub public key before we will send a heartbeat
			if hubURI != "" && key != "" {

				var object = fmt.Sprintf(`{
						"key" : "%s",
						"version" : "%s",
						"hub_encryption": "%s",
						"e2e_encryption": "%s",
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
					}`, config.Key, kerberosAgentVersion, hub_encryption, e2e_encryption, system.Version, system.CPUId, username, key, name, isEnterprise, system.Hostname, system.Architecture, system.TotalMemory, system.UsedMemory, system.FreeMemory, system.ProcessUsedMemory, macs, ips, "0", "0", "0", uptimeString, boottimeString, config.HubSite, onvifEnabled, onvifZoom, onvifPanTilt, onvifPresets, onvifPresetsList, onvifEventsList, cameraConnected, hasBackChannel)

				// Get the private key to encrypt the data using symmetric encryption: AES.
				privateKey := config.HubPrivateKey
				if hub_encryption == "true" && privateKey != "" {
					// Encrypt the data using AES.
					encrypted, err := encryption.AesEncrypt([]byte(object), privateKey)
					if err != nil {
						encrypted = []byte("")
						log.Log.Error("cloud.HandleHeartBeat(): error while encrypting data: " + err.Error())
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
					log.Log.Info("cloud.HandleHeartBeat(): (200) Heartbeat received by Kerberos Hub.")
				} else {
					if communication.CloudTimestamp != nil && communication.CloudTimestamp.Load() != nil {
						communication.CloudTimestamp.Store(int64(0))
					}
					log.Log.Error("cloud.HandleHeartBeat(): (400) Something went wrong while sending to Kerberos Hub.")
				}
			} else {
				log.Log.Error("cloud.HandleHeartBeat(): Disabled as we do not have a public key defined.")
			}

			// If we have a Kerberos Vault connected, we will also send some analytics
			// to that service.
			vaultURI = config.KStorage.URI
			accessKey := config.KStorage.AccessKey
			secretAccessKey := config.KStorage.SecretAccessKey
			if vaultURI != "" && accessKey != "" && secretAccessKey != "" {

				var object = fmt.Sprintf(`{
					"key" : "%s",
					"version" : "%s",
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
				}`, config.Key, kerberosAgentVersion, system.Version, system.CPUId, username, key, name, isEnterprise, system.Hostname, system.Architecture, system.TotalMemory, system.UsedMemory, system.FreeMemory, system.ProcessUsedMemory, macs, ips, "0", "0", "0", uptimeString, boottimeString, config.HubSite, onvifEnabled, onvifZoom, onvifPanTilt, onvifPresets, onvifPresetsList, cameraConnected)

				var jsonStr = []byte(object)
				buffy := bytes.NewBuffer(jsonStr)
				req, _ := http.NewRequest("POST", vaultURI+"/devices/heartbeat", buffy)
				req.Header.Set("Content-Type", "application/json")

				resp, err := client.Do(req)
				if resp != nil {
					resp.Body.Close()
				}
				if err == nil && resp.StatusCode == 200 {
					log.Log.Info("cloud.HandleHeartBeat(): (200) Heartbeat received by Kerberos Vault.")
				} else {
					log.Log.Error("cloud.HandleHeartBeat(): (400) Something went wrong while sending to Kerberos Vault.")
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

	if pullPointAddressLoopState != "" {
		cameraConfiguration := configuration.Config.Capture.IPCamera
		device, _, err := onvif.ConnectToOnvifDevice(&cameraConfiguration)
		if err != nil {
			onvif.UnsubscribePullPoint(device, pullPointAddressLoopState)
		}
	}

	log.Log.Debug("cloud.HandleHeartBeat(): finished")
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

			deviceId := config.Key
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

					chunking := config.Capture.LiveviewChunking

					if chunking == "true" {

						// Split encoded image into chunks of 2kb
						// This is to prevent the MQTT message to be too large.
						// By default, bytes are not encoded to base64 here; you are splitting the raw JPEG/PNG bytes.
						// However, in MQTT and web contexts, binary data may not be handled well, so base64 is often used.
						// To avoid base64 encoding, just send the raw []byte chunks as you do here.
						// If you want to avoid base64, make sure the receiver can handle binary payloads.

						chunkSize := 25 * 1024 // 25KB chunks
						var chunks [][]byte
						for i := 0; i < len(bytes); i += chunkSize {
							end := i + chunkSize
							if end > len(bytes) {
								end = len(bytes)
							}
							chunk := bytes[i:end]
							chunks = append(chunks, chunk)
						}

						log.Log.Infof("cloud.HandleLiveStreamSD(): Sending %d chunks of size %d bytes.", len(chunks), chunkSize)

						timestamp := time.Now().Unix()
						for i, chunk := range chunks {
							valueMap := make(map[string]interface{})
							valueMap["id"] = timestamp
							valueMap["chunk"] = chunk
							valueMap["chunkIndex"] = i
							valueMap["chunkSize"] = chunkSize
							valueMap["chunkCount"] = len(chunks)
							message := models.Message{
								Payload: models.Payload{
									Version:  "v1.0.0",
									Action:   "receive-sd-stream",
									DeviceId: deviceId,
									Value:    valueMap,
								},
							}
							payload, err := models.PackageMQTTMessage(configuration, message)
							if err == nil {
								mqttClient.Publish("kerberos/hub/"+hubKey+"/"+deviceId, 1, false, payload)
								log.Log.Infof("cloud.HandleLiveStreamSD(): sent chunk %d/%d to MQTT topic kerberos/hub/%s/%s", i+1, len(chunks), hubKey, deviceId)
								time.Sleep(33 * time.Millisecond) // Sleep to avoid flooding the MQTT broker with messages
							} else {
								log.Log.Info("cloud.HandleLiveStreamSD(): something went wrong while sending acknowledge config to hub: " + string(payload))
							}
						}
					} else {

						valueMap := make(map[string]interface{})
						valueMap["image"] = bytes
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
				time.Sleep(1000 * time.Millisecond) // Sleep to avoid flooding the MQTT broker with messages
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
		log.Log.Debug("cloud.HandleLiveStreamHD(): stopping as Offline is enabled.")
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
				log.Log.Info("cloud.HandleLiveStreamHD(): Waiting for peer connections.")
				for handshake := range communication.HandleLiveHDHandshake {
					log.Log.Info("cloud.HandleLiveStreamHD(): setting up a peer connection.")
					go webrtc.InitializeWebRTCConnection(configuration, communication, mqttClient, videoTrack, audioTrack, handshake)
				}
			}

		} else {
			log.Log.Debug("cloud.HandleLiveStreamHD(): stopping as Liveview is disabled.")
		}
	}
}

func HandleRealtimeProcessing(processingCursor *packets.QueueCursor, configuration *models.Configuration, communication *models.Communication, mqttClient mqtt.Client, rtspClient capture.RTSPClient) {

	log.Log.Debug("cloud.RealtimeProcessing(): started")

	config := configuration.Config

	// If offline made is enabled, we will stop the thread.
	if config.Offline == "true" {
		log.Log.Debug("cloud.RealtimeProcessing(): stopping as Offline is enabled.")
	} else {

		// Check if we need to enable the realtime processing
		if config.RealtimeProcessing == "true" {

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

			// We will publish the keyframes to the MQTT topic.
			realtimeProcessingTopic := "kerberos/keyframes/" + hubKey
			if config.RealtimeProcessingTopic != "" {
				realtimeProcessingTopic = config.RealtimeProcessingTopic
			}

			var cursorError error
			var pkt packets.Packet

			for cursorError == nil {
				pkt, cursorError = processingCursor.ReadPacket()
				if len(pkt.Data) == 0 || !pkt.IsKeyFrame {
					continue
				}

				log.Log.Info("cloud.RealtimeProcessing(): Sending base64 encoded images to MQTT.")
				img, err := rtspClient.DecodePacket(pkt)
				if err == nil {
					bytes, _ := utils.ImageToBytes(&img)
					encoded := base64.StdEncoding.EncodeToString(bytes)

					valueMap := make(map[string]interface{})
					valueMap["image"] = encoded
					message := models.Message{
						Payload: models.Payload{
							Action:   "receive-keyframe",
							DeviceId: configuration.Config.Key,
							Value:    valueMap,
						},
					}
					payload, err := models.PackageMQTTMessage(configuration, message)
					if err == nil {
						mqttClient.Publish(realtimeProcessingTopic, 0, false, payload)
					} else {
						log.Log.Info("cloud.RealtimeProcessing(): something went wrong while sending acknowledge config to hub: " + string(payload))
					}
				}
			}

		} else {
			log.Log.Debug("cloud.RealtimeProcessing(): stopping as Liveview is disabled.")
		}
	}

	log.Log.Debug("cloud.HandleLiveStreamSD(): finished")
}

// VerifyHub godoc
// @Router /api/hub/verify [post]
// @ID verify-hub
// @Security Bearer
// @securityDefinitions.apikey Bearer
// @in header
// @name Authorization
// @Tags persistence
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
				body, err := io.ReadAll(resp.Body)
				defer resp.Body.Close()
				if err == nil {
					if resp.StatusCode == 200 {
						c.JSON(200, body)
					} else {
						c.JSON(400, models.APIResponse{
							Data: "cloud.VerifyHub(): something went wrong while reaching the Kerberos Hub API: " + string(body),
						})
					}
				} else {
					c.JSON(400, models.APIResponse{
						Data: "cloud.VerifyHub(): something went wrong while ready the response body: " + err.Error(),
					})
				}
			} else {
				c.JSON(400, models.APIResponse{
					Data: "cloud.VerifyHub(): something went wrong while reaching to the Kerberos Hub API: " + hubURI,
				})
			}
		} else {
			c.JSON(400, models.APIResponse{
				Data: "cloud.VerifyHub(): something went wrong while creating the HTTP request: " + err.Error(),
			})
		}
	} else {
		c.JSON(400, models.APIResponse{
			Data: "cloud.VerifyHub(): something went wrong while receiving the config " + err.Error(),
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
// @Tags persistence
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
				msg := "cloud.VerifyPersistence(kerberoshub): Kerberos Hub not properly configured."
				log.Log.Error(msg)
				c.JSON(400, models.APIResponse{
					Data: msg,
				})
			} else {

				// Open test-480p.mp4
				file, err := os.Open(configDirectory + "/data/test-480p.mp4")
				if err != nil {
					msg := "cloud.VerifyPersistence(kerberoshub): error reading test-480p.mp4: " + err.Error()
					log.Log.Error(msg)
					c.JSON(400, models.APIResponse{
						Data: msg,
					})
				}
				defer file.Close()

				req, err := http.NewRequest("POST", config.HubURI+"/storage/upload", file)
				if err != nil {
					msg := "cloud.VerifyPersistence(kerberoshub): error reading Kerberos Hub HEAD request, " + config.HubURI + "/storage: " + err.Error()
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
						msg := "cloud.VerifyPersistence(kerberoshub): Upload allowed using the credentials provided (" + config.HubKey + ", " + config.HubPrivateKey + ")"
						log.Log.Info(msg)
						c.JSON(200, models.APIResponse{
							Data: msg,
						})
					} else {
						msg := "cloud.VerifyPersistence(kerberoshub): Upload NOT allowed using the credentials provided (" + config.HubKey + ", " + config.HubPrivateKey + ")"
						log.Log.Error(msg)
						c.JSON(400, models.APIResponse{
							Data: msg,
						})
					}
				} else {
					msg := "cloud.VerifyPersistence(kerberoshub): Error creating Kerberos Hub request"
					log.Log.Error(msg)
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
				if err == nil {
					req.Header.Add("X-Kerberos-Storage-AccessKey", accessKey)
					req.Header.Add("X-Kerberos-Storage-SecretAccessKey", secretAccessKey)
					resp, err := client.Do(req)

					if err == nil {
						body, err := io.ReadAll(resp.Body)
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
									msg := "cloud.VerifyPersistence(kerberosvault): error reading test-480p.mp4: " + err.Error()
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
											body, err := io.ReadAll(resp.Body)
											defer resp.Body.Close()
											if err == nil {
												if resp.StatusCode == 200 {
													msg := "cloud.VerifyPersistence(kerberosvault): Upload allowed using the credentials provided (" + accessKey + ", " + secretAccessKey + ")"
													log.Log.Info(msg)
													c.JSON(200, models.APIResponse{
														Data: body,
													})
												} else {
													msg := "cloud.VerifyPersistence(kerberosvault): Something went wrong while verifying your persistence settings. Make sure your provider is the same as the storage provider in your Kerberos Vault, and the relevant storage provider is configured properly."
													log.Log.Error(msg)
													c.JSON(400, models.APIResponse{
														Data: msg,
													})
												}
											}
										}
									} else {
										msg := "cloud.VerifyPersistence(kerberosvault): Upload of fake recording failed: " + err.Error()
										log.Log.Error(msg)
										c.JSON(400, models.APIResponse{
											Data: msg,
										})
									}
								} else {
									msg := "cloud.VerifyPersistence(kerberosvault): Something went wrong while creating /storage POST request." + err.Error()
									log.Log.Error(msg)
									c.JSON(400, models.APIResponse{
										Data: msg,
									})
								}
							} else {
								msg := "cloud.VerifyPersistence(kerberosvault): Provider and/or directory is missing from the request."
								log.Log.Error(msg)
								c.JSON(400, models.APIResponse{
									Data: msg,
								})
							}
						} else {
							msg := "cloud.VerifyPersistence(kerberosvault): Something went wrong while verifying storage credentials: " + string(body)
							log.Log.Error(msg)
							c.JSON(400, models.APIResponse{
								Data: msg,
							})
						}
					} else {
						msg := "cloud.VerifyPersistence(kerberosvault): Something went wrong while verifying storage credentials:" + err.Error()
						log.Log.Error(msg)
						c.JSON(400, models.APIResponse{
							Data: msg,
						})
					}
				} else {
					msg := "cloud.VerifyPersistence(kerberosvault): Something went wrong while verifying storage credentials:" + err.Error()
					log.Log.Error(msg)
					c.JSON(400, models.APIResponse{
						Data: msg,
					})
				}
			} else {
				msg := "cloud.VerifyPersistence(kerberosvault): please fill-in the required Kerberos Vault credentials."
				log.Log.Error(msg)
				c.JSON(400, models.APIResponse{
					Data: msg,
				})
			}
		}
	} else {
		msg := "cloud.VerifyPersistence(): No persistence was specified, so do not know what to verify:" + err.Error()
		log.Log.Error(msg)
		c.JSON(400, models.APIResponse{
			Data: msg,
		})
	}
}

// VerifySecondaryPersistence godoc
// @Router /api/persistence/secondary/verify [post]
// @ID verify-persistence
// @Security Bearer
// @securityDefinitions.apikey Bearer
// @in header
// @name Authorization
// @Tags persistence
// @Param config body models.Config true "Config"
// @Summary Will verify the secondary persistence.
// @Description Will verify the secondary persistence.
// @Success 200 {object} models.APIResponse
func VerifySecondaryPersistence(c *gin.Context, configDirectory string) {

	var config models.Config
	err := c.BindJSON(&config)
	if err != nil || config.Cloud != "" {

		if config.Cloud == "kstorage" || config.Cloud == "kerberosvault" {

			if config.KStorageSecondary == nil {
				msg := "cloud.VerifySecondaryPersistence(kerberosvault): please fill-in the required Kerberos Vault credentials."
				log.Log.Error(msg)
				c.JSON(400, models.APIResponse{
					Data: msg,
				})

			} else {

				uri := config.KStorageSecondary.URI
				accessKey := config.KStorageSecondary.AccessKey
				secretAccessKey := config.KStorageSecondary.SecretAccessKey
				directory := config.KStorageSecondary.Directory
				provider := config.KStorageSecondary.Provider

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
					if err == nil {
						req.Header.Add("X-Kerberos-Storage-AccessKey", accessKey)
						req.Header.Add("X-Kerberos-Storage-SecretAccessKey", secretAccessKey)
						resp, err := client.Do(req)

						if err == nil {
							body, err := io.ReadAll(resp.Body)
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
										msg := "cloud.VerifyPersistence(kerberosvault): error reading test-480p.mp4: " + err.Error()
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
												body, err := io.ReadAll(resp.Body)
												defer resp.Body.Close()
												if err == nil {
													if resp.StatusCode == 200 {
														msg := "cloud.VerifySecondaryPersistence(kerberosvault): Upload allowed using the credentials provided (" + accessKey + ", " + secretAccessKey + ")"
														log.Log.Info(msg)
														c.JSON(200, models.APIResponse{
															Data: body,
														})
													} else {
														msg := "cloud.VerifySecondaryPersistence(kerberosvault): Something went wrong while verifying your persistence settings. Make sure your provider is the same as the storage provider in your Kerberos Vault, and the relevant storage provider is configured properly."
														log.Log.Error(msg)
														c.JSON(400, models.APIResponse{
															Data: msg,
														})
													}
												}
											}
										} else {
											msg := "cloud.VerifySecondaryPersistence(kerberosvault): Upload of fake recording failed: " + err.Error()
											log.Log.Error(msg)
											c.JSON(400, models.APIResponse{
												Data: msg,
											})
										}
									} else {
										msg := "cloud.VerifySecondaryPersistence(kerberosvault): Something went wrong while creating /storage POST request." + err.Error()
										log.Log.Error(msg)
										c.JSON(400, models.APIResponse{
											Data: msg,
										})
									}
								} else {
									msg := "cloud.VerifySecondaryPersistence(kerberosvault): Provider and/or directory is missing from the request."
									log.Log.Error(msg)
									c.JSON(400, models.APIResponse{
										Data: msg,
									})
								}
							} else {
								msg := "cloud.VerifySecondaryPersistence(kerberosvault): Something went wrong while verifying storage credentials: " + string(body)
								log.Log.Error(msg)
								c.JSON(400, models.APIResponse{
									Data: msg,
								})
							}
						} else {
							msg := "cloud.VerifySecondaryPersistence(kerberosvault): Something went wrong while verifying storage credentials:" + err.Error()
							log.Log.Error(msg)
							c.JSON(400, models.APIResponse{
								Data: msg,
							})
						}
					} else {
						msg := "cloud.VerifySecondaryPersistence(kerberosvault): Something went wrong while verifying storage credentials:" + err.Error()
						log.Log.Error(msg)
						c.JSON(400, models.APIResponse{
							Data: msg,
						})
					}
				} else {
					msg := "cloud.VerifySecondaryPersistence(kerberosvault): please fill-in the required Kerberos Vault credentials."
					log.Log.Error(msg)
					c.JSON(400, models.APIResponse{
						Data: msg,
					})
				}
			}
		}
	} else {
		msg := "cloud.VerifySecondaryPersistence(): No persistence was specified, so do not know what to verify:" + err.Error()
		log.Log.Error(msg)
		c.JSON(400, models.APIResponse{
			Data: msg,
		})
	}
}
