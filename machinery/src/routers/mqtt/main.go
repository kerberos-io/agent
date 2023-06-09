package mqtt

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/webrtc"
)

// We'll cache the MQTT settings to know if we need to reinitialize the MQTT client connection.
// If we update the configuration but no new MQTT settings are provided, we don't need to restart it.
var PREV_MQTTURI string
var PREV_MQTTUsername string
var PREV_MQTTPassword string
var PREV_HubKey string
var PREV_AgentKey string

func HasMQTTClientModified(configuration *models.Configuration) bool {
	MTTURI := configuration.Config.MQTTURI
	MTTUsername := configuration.Config.MQTTUsername
	MQTTPassword := configuration.Config.MQTTPassword
	HubKey := configuration.Config.HubKey
	AgentKey := configuration.Config.Key
	if PREV_MQTTURI != MTTURI || PREV_MQTTUsername != MTTUsername || PREV_MQTTPassword != MQTTPassword || PREV_HubKey != HubKey || PREV_AgentKey != AgentKey {
		log.Log.Info("HasMQTTClientModified: MQTT settings have been modified, restarting MQTT client.")
		return true
	}
	return false
}

func ConfigureMQTT(configuration *models.Configuration, communication *models.Communication) mqtt.Client {

	config := configuration.Config

	// Set the MQTT settings.
	PREV_MQTTURI = configuration.Config.MQTTURI
	PREV_MQTTUsername = configuration.Config.MQTTUsername
	PREV_MQTTPassword = configuration.Config.MQTTPassword
	PREV_HubKey = configuration.Config.HubKey
	PREV_AgentKey = configuration.Config.Key

	if config.Offline == "true" {
		log.Log.Info("ConfigureMQTT: not starting as running in Offline mode.")
	} else {

		opts := mqtt.NewClientOptions()

		// We will set the MQTT endpoint to which we want to connect
		// and share and receive messages to/from.
		mqttURL := config.MQTTURI
		opts.AddBroker(mqttURL)
		log.Log.Info("ConfigureMQTT: Set broker uri " + mqttURL)

		// Our MQTT broker can have username/password credentials
		// to protect it from the outside.
		mqtt_username := config.MQTTUsername
		mqtt_password := config.MQTTPassword
		if mqtt_username != "" || mqtt_password != "" {
			opts.SetUsername(mqtt_username)
			opts.SetPassword(mqtt_password)
			log.Log.Info("ConfigureMQTT: Set username " + mqtt_username)
			log.Log.Info("ConfigureMQTT: Set password " + mqtt_password)
		}

		// Some extra options to make sure the connection behaves
		// properly. More information here: github.com/eclipse/paho.mqtt.golang.
		opts.SetCleanSession(true)
		opts.SetConnectRetry(true)
		//opts.SetAutoReconnect(true)
		opts.SetConnectTimeout(30 * time.Second)

		hubKey := ""
		// This is the old way ;)
		if config.Cloud == "s3" && config.S3 != nil && config.S3.Publickey != "" {
			hubKey = config.S3.Publickey
		} else if config.Cloud == "kstorage" && config.KStorage != nil && config.KStorage.CloudKey != "" {
			hubKey = config.KStorage.CloudKey
		}
		// This is the new way ;)
		if config.HubKey != "" {
			hubKey = config.HubKey
		}

		if hubKey != "" {

			rand.Seed(time.Now().UnixNano())
			random := rand.Intn(100)
			mqttClientID := config.Key + strconv.Itoa(random) // this random int is to avoid conflicts.

			// This is a worked-around.
			// current S3 (Kerberos Hub SAAS) is using a secured MQTT, where the client id,
			// should match the kerberos hub key.
			if config.Cloud == "s3" {
				mqttClientID = config.Key
			}

			opts.SetClientID(mqttClientID)
			log.Log.Info("ConfigureMQTT: Set ClientID " + mqttClientID)
			rand.Seed(time.Now().UnixNano())
			webrtc.CandidateArrays = make(map[string](chan string))

			opts.OnConnect = func(c mqtt.Client) {
				// We managed to connect to the MQTT broker, hurray!
				log.Log.Info("ConfigureMQTT: " + mqttClientID + " connected to " + mqttURL)

				// Create a subscription to know if send out a livestream or not.
				MQTTListenerHandleLiveSD(c, hubKey, configuration, communication)

				// Create a subscription for the WEBRTC livestream.
				MQTTListenerHandleLiveHDHandshake(c, hubKey, configuration, communication)

				// Create a subscription for keeping alive the WEBRTC livestream.
				MQTTListenerHandleLiveHDKeepalive(c, hubKey, configuration, communication)

				// Create a subscription to listen to the number of WEBRTC peers.
				MQTTListenerHandleLiveHDPeers(c, hubKey, configuration, communication)

				// Create a subscription to listen for WEBRTC candidates.
				MQTTListenerHandleLiveHDCandidates(c, hubKey, configuration, communication)

				// Create a susbcription to listen for ONVIF actions: e.g. PTZ, Zoom, etc.
				MQTTListenerHandleONVIF(c, hubKey, configuration, communication)
			}
		}
		mqc := mqtt.NewClient(opts)
		if token := mqc.Connect(); token.WaitTimeout(3 * time.Second) {
			if token.Error() != nil {
				log.Log.Error("ConfigureMQTT: unable to establish mqtt broker connection, error was: " + token.Error().Error())
			}
		}
		return mqc
	}

	return nil
}

func MQTTListenerHandleLiveSD(mqttClient mqtt.Client, hubKey string, configuration *models.Configuration, communication *models.Communication) {
	config := configuration.Config
	topicRequest := "kerberos/" + hubKey + "/device/" + config.Key + "/request-live"
	mqttClient.Subscribe(topicRequest, 0, func(c mqtt.Client, msg mqtt.Message) {
		if communication.CameraConnected {
			select {
			case communication.HandleLiveSD <- time.Now().Unix():
			default:
			}
			log.Log.Info("MQTTListenerHandleLiveSD: received request to livestream.")
		} else {
			log.Log.Info("MQTTListenerHandleLiveSD: received request to livestream, but camera is not connected.")
		}
		msg.Ack()
	})
}

func MQTTListenerHandleLiveHDHandshake(mqttClient mqtt.Client, hubKey string, configuration *models.Configuration, communication *models.Communication) {
	config := configuration.Config
	topicRequestWebRtc := config.Key + "/register"
	mqttClient.Subscribe(topicRequestWebRtc, 0, func(c mqtt.Client, msg mqtt.Message) {
		if communication.CameraConnected {
			var sdp models.SDPPayload
			json.Unmarshal(msg.Payload(), &sdp)
			select {
			case communication.HandleLiveHDHandshake <- sdp:
			default:
			}
			log.Log.Info("MQTTListenerHandleLiveHDHandshake: received request to setup webrtc.")
		} else {
			log.Log.Info("MQTTListenerHandleLiveHDHandshake: received request to setup webrtc, but camera is not connected.")
		}
		msg.Ack()
	})
}

func MQTTListenerHandleLiveHDKeepalive(mqttClient mqtt.Client, hubKey string, configuration *models.Configuration, communication *models.Communication) {
	config := configuration.Config
	topicKeepAlive := fmt.Sprintf("kerberos/webrtc/keepalivehub/%s", config.Key)
	mqttClient.Subscribe(topicKeepAlive, 0, func(c mqtt.Client, msg mqtt.Message) {
		if communication.CameraConnected {
			alive := string(msg.Payload())
			communication.HandleLiveHDKeepalive <- alive
			log.Log.Info("MQTTListenerHandleLiveHDKeepalive: Received keepalive: " + alive)
		} else {
			log.Log.Info("MQTTListenerHandleLiveHDKeepalive: received keepalive, but camera is not connected.")
		}
	})
}

func MQTTListenerHandleLiveHDPeers(mqttClient mqtt.Client, hubKey string, configuration *models.Configuration, communication *models.Communication) {
	config := configuration.Config
	topicPeers := fmt.Sprintf("kerberos/webrtc/peers/%s", config.Key)
	mqttClient.Subscribe(topicPeers, 0, func(c mqtt.Client, msg mqtt.Message) {
		if communication.CameraConnected {
			peerCount := string(msg.Payload())
			communication.HandleLiveHDPeers <- peerCount
			log.Log.Info("MQTTListenerHandleLiveHDPeers: Number of peers listening: " + peerCount)
		} else {
			log.Log.Info("MQTTListenerHandleLiveHDPeers: received peer count, but camera is not connected.")
		}
	})
}

func MQTTListenerHandleLiveHDCandidates(mqttClient mqtt.Client, hubKey string, configuration *models.Configuration, communication *models.Communication) {
	config := configuration.Config
	topicCandidates := "candidate/cloud"
	mqttClient.Subscribe(topicCandidates, 0, func(c mqtt.Client, msg mqtt.Message) {
		if communication.CameraConnected {
			var candidate models.Candidate
			json.Unmarshal(msg.Payload(), &candidate)
			if candidate.CloudKey == config.Key {
				key := candidate.CloudKey + "/" + candidate.Cuuid
				candidatesExists := false
				var channel chan string
				for !candidatesExists {
					webrtc.CandidatesMutex.Lock()
					channel, candidatesExists = webrtc.CandidateArrays[key]
					webrtc.CandidatesMutex.Unlock()
				}
				log.Log.Info("MQTTListenerHandleLiveHDCandidates: " + string(msg.Payload()))
				channel <- string(msg.Payload())
			}
		} else {
			log.Log.Info("MQTTListenerHandleLiveHDCandidates: received candidate, but camera is not connected.")
		}
	})
}

func MQTTListenerHandleONVIF(mqttClient mqtt.Client, hubKey string, configuration *models.Configuration, communication *models.Communication) {
	config := configuration.Config
	topicOnvif := fmt.Sprintf("kerberos/onvif/%s", config.Key)
	mqttClient.Subscribe(topicOnvif, 0, func(c mqtt.Client, msg mqtt.Message) {
		if communication.CameraConnected {
			var onvifAction models.OnvifAction
			json.Unmarshal(msg.Payload(), &onvifAction)
			communication.HandleONVIF <- onvifAction
			log.Log.Info("MQTTListenerHandleONVIF: Received an action - " + onvifAction.Action)
		} else {
			log.Log.Info("MQTTListenerHandleONVIF: received action, but camera is not connected.")
		}
	})
}

func DisconnectMQTT(mqttClient mqtt.Client, config *models.Config) {
	if mqttClient != nil {
		// Cleanup all subscriptions
		mqttClient.Unsubscribe("kerberos/" + PREV_HubKey + "/device/" + PREV_AgentKey + "/request-live")
		mqttClient.Unsubscribe(PREV_AgentKey + "/register")
		mqttClient.Unsubscribe("kerberos/webrtc/keepalivehub/" + PREV_AgentKey)
		mqttClient.Unsubscribe("kerberos/webrtc/peers/" + PREV_AgentKey)
		mqttClient.Unsubscribe("candidate/cloud")
		mqttClient.Unsubscribe("kerberos/onvif/" + PREV_AgentKey)
		mqttClient.Disconnect(1000)
		mqttClient = nil
		log.Log.Info("DisconnectMQTT: MQTT client disconnected.")
	}
}
