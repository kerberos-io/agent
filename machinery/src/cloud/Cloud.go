package cloud

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/kerberos-io/joy4/av/pubsub"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	av "github.com/kerberos-io/joy4/av"
	"github.com/kerberos-io/joy4/cgo/ffmpeg"
	"gocv.io/x/gocv"

	"net/http"
	"runtime"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/kerberos-io/agent/machinery/src/computervision"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/utils"
	"github.com/kerberos-io/agent/machinery/src/webrtc"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/host"
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

loop:
	for {
		ff, err := utils.ReadDirectory(watchDirectory)

		// This will check if we need to stop the thread,
		// because of a reconfiguration.
		select {
		case <-communication.HandleUpload:
			break loop
		default:
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
		time.Sleep(1 * time.Second)
	}

	log.Log.Debug("HandleUpload: finished")
}

func HandleHeartBeat(configuration *models.Configuration, communication *models.Communication) {

	log.Log.Debug("HandleHeartBeat: started")

	config := configuration.Config

	url := config.HeartbeatURI
	key := ""
	username := ""
	vaultURI := ""

	if config.Cloud == "s3" && config.S3.Publickey != "" {
		username = config.S3.Username
		key = config.S3.Publickey
	} else if config.Cloud == "kstorage" && config.KStorage.CloudKey != "" {
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
		// This will check if we need to stop the thread,
		// because of a reconfiguration.
		select {
		case <-communication.HandleHeartBeat:
			break loop
		default:
		}

		uptime, _ := host.Uptime()
		days := strconv.Itoa(int(uptime / (60 * 60 * 24)))
		//12:11:48 up 11 days

		//partitions, _ := disk.Partitions(false)
		usage, _ := disk.Usage("/")
		diskPercentUsed := strconv.Itoa(int(usage.UsedPercent))

		onvifEnabled := "false"
		if config.Capture.IPCamera.ONVIFXAddr != "" {
			onvifEnabled = "true"
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
			"enterprise" : true,
			"board" : "",
			"disk1size" : "%s",
			"disk3size" : "%s",
			"diskvdasize" :  "%s",
			"numberoffiles" : "33",
			"temperature" : "sh: 1: vcgencmd: not found",
			"wifissid" : "",
			"wifistrength" : "",
			"uptime" : "up %s days,",
			"timestamp" : 1564747908,
			"siteID" : "%s",
			"onvif" : "%s"
		}`, config.Key, username, key, config.Name, "0", "0", diskPercentUsed, days, config.HubSite, onvifEnabled)

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

		time.Sleep(30 * time.Second)
	}

	log.Log.Debug("HandleHeartBeat: finished")
}

func HandleLiveStreamSD(livestreamCursor *pubsub.QueueCursor, configuration *models.Configuration, communication *models.Communication, mqttClient mqtt.Client, decoder *ffmpeg.VideoDecoder, decoderMutex *sync.Mutex) {

	log.Log.Debug("HandleLiveStreamSD: finished")

	config := configuration.Config
	key := ""
	if config.Cloud == "s3" && config.S3.Publickey != "" {
		key = config.S3.Publickey
	} else if config.Cloud == "kstorage" && config.KStorage.CloudKey != "" {
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
			_, ok := webrtc.CandidateArrays[key]
			if !ok {
				webrtc.CandidateArrays[key] = make(chan string, 30)
			}
			webrtc.InitializeWebRTCConnection(configuration, communication, mqttClient, track, handshake, webrtc.CandidateArrays[key])
		}
	}
}
