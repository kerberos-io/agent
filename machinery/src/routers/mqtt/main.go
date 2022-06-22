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

func ConfigureMQTT(configuration *models.Configuration, communication *models.Communication) mqtt.Client {

	config := configuration.Config

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
	opts.SetConnectTimeout(30 * time.Second)

	hubKey := ""
	// This is the old way ;)
	if config.Cloud == "s3" && config.S3.Publickey != "" {
		hubKey = config.S3.Publickey
	} else if config.Cloud == "kstorage" && config.KStorage.CloudKey != "" {
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

func MQTTListenerHandleLiveSD(mqttClient mqtt.Client, hubKey string, configuration *models.Configuration, communication *models.Communication) {
	config := configuration.Config
	topicRequest := "kerberos/" + hubKey + "/device/" + config.Key + "/request-live"
	mqttClient.Subscribe(topicRequest, 0, func(c mqtt.Client, msg mqtt.Message) {
		select {
		case communication.HandleLiveSD <- time.Now().Unix():
		default:
		}
		log.Log.Info("MQTTListenerHandleLiveSD: received request to livestream.")
		msg.Ack()
	})
}

func MQTTListenerHandleLiveHDHandshake(mqttClient mqtt.Client, hubKey string, configuration *models.Configuration, communication *models.Communication) {
	config := configuration.Config
	topicRequestWebRtc := config.Key + "/register"
	mqttClient.Subscribe(topicRequestWebRtc, 0, func(c mqtt.Client, msg mqtt.Message) {
		log.Log.Info("MQTTListenerHandleLiveHDHandshake: received request to setup webrtc.")
		var sdp models.SDPPayload
		json.Unmarshal(msg.Payload(), &sdp)
		select {
		case communication.HandleLiveHDHandshake <- sdp:
		default:
		}
		msg.Ack()
	})
}

func MQTTListenerHandleLiveHDKeepalive(mqttClient mqtt.Client, hubKey string, configuration *models.Configuration, communication *models.Communication) {
	config := configuration.Config
	topicKeepAlive := fmt.Sprintf("kerberos/webrtc/keepalivehub/%s", config.Key)
	mqttClient.Subscribe(topicKeepAlive, 0, func(c mqtt.Client, msg mqtt.Message) {
		alive := string(msg.Payload())
		communication.HandleLiveHDKeepalive <- alive
		log.Log.Info("MQTTListenerHandleLiveHDKeepalive: Received keepalive: " + alive)
	})
}

func MQTTListenerHandleLiveHDPeers(mqttClient mqtt.Client, hubKey string, configuration *models.Configuration, communication *models.Communication) {
	config := configuration.Config
	topicPeers := fmt.Sprintf("kerberos/webrtc/peers/%s", config.Key)
	mqttClient.Subscribe(topicPeers, 0, func(c mqtt.Client, msg mqtt.Message) {
		peerCount := string(msg.Payload())
		communication.HandleLiveHDPeers <- peerCount
		log.Log.Info("MQTTListenerHandleLiveHDPeers: Number of peers listening: " + peerCount)
	})
}

func MQTTListenerHandleONVIF(mqttClient mqtt.Client, hubKey string, configuration *models.Configuration, communication *models.Communication) {
	config := configuration.Config
	topicOnvif := fmt.Sprintf("kerberos/onvif/%s", config.Key)
	mqttClient.Subscribe(topicOnvif, 0, func(c mqtt.Client, msg mqtt.Message) {
		var onvifAction models.OnvifAction
		json.Unmarshal(msg.Payload(), &onvifAction)
		communication.HandleONVIF <- onvifAction
		log.Log.Info("MQTTListenerHandleONVIF: Received an action - " + onvifAction.Action)
	})
}

func DisconnectMQTT(mqttClient mqtt.Client) {
	mqttClient.Disconnect(1000)
}
