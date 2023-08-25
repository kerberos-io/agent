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
	"github.com/kerberos-io/agent/machinery/src/onvif"
	"github.com/kerberos-io/agent/machinery/src/webrtc"
)

// The message structure which is used to send over
// and receive messages from the MQTT broker
type Message struct {
	Mid         string  `json:"mid"`
	Timestamp   int64   `json:"timestamp"`
	Encrypted   bool    `json:"encrypted"`
	PublicKey   string  `json:"public_key"`
	Fingerprint string  `json:"fingerprint"`
	Payload     Payload `json:"payload"`
}

// The payload structure which is used to send over
// and receive messages from the MQTT broker
type Payload struct {
	Action   string                 `json:"action"`
	DeviceId string                 `json:"device_id"`
	Value    map[string]interface{} `json:"value"`
}

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

func PackageMQTTMessage(msg Message) ([]byte, error) {
	// We'll generate an unique id, and encrypt it using the private key.
	msg.Mid = "0123456789+1"
	msg.Timestamp = time.Now().Unix()
	msg.Encrypted = false
	msg.PublicKey = ""
	msg.Fingerprint = ""
	payload, err := json.Marshal(msg)
	return payload, err
}

// Configuring MQTT to subscribe for various bi-directional messaging
// Listen and reply (a generic method to share and retrieve information)
//
// !!! NEW METHOD TO COMMUNICATE: only create a single subscription for all communication.
// and an additional publish messages back
//
// - [SUBSCRIPTION] kerberos/agent/{hubkey} 		(hub -> agent)
// - [PUBLISH] kerberos/hub/{hubkey}  		(agent -> hub)
//
// !!! LEGACY METHODS BELOW, WE SHOULD LEVERAGE THE ABOVE METHOD!
//
// [SUBSCRIPTIONS]
//
// SD Streaming (Base64 JPEGs)
// - kerberos/{hubkey}/device/{devicekey}/request-live: use for polling of SD live streaming (as long the user requests stream, we'll send JPEGs over).
//
// HD Streaming (WebRTC)
// - kerberos/register: use for receiving HD live streaming requests.
// - candidate/cloud: remote ICE candidates are shared over this line.
// - kerberos/webrtc/keepalivehub/{devicekey}: use for polling of HD streaming (as long the user requests stream, we'll send it over).
// - kerberos/webrtc/peers/{devicekey}: we'll keep track of the number of peers (we can have more than 1 concurrent listeners).
//
// ONVIF capabilities
// - kerberos/onvif/{devicekey}: endpoint to execute ONVIF commands such as (PTZ, Zoom, IO, etc)
//
// [PUBlISH]
// Next to subscribing to various topics, we'll also publish messages to various topics, find a list of available Publish methods.
//
// - kerberos/webrtc/packets/{devicekey}: use for forwarding WebRTC (RTP Packets) over MQTT -> Complex firewall.
// - kerberos/webrtc/keepalive/{devicekey}: use for keeping alive forwarded WebRTC stream
// - {devicekey}/{sessionid}/answer: once a WebRTC request is received through (kerberos/register), we'll draft an answer and send it back to the remote WebRTC client.
// - kerberos/{hubkey}/device/{devicekey}/motion: a motion signal

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

				// Create a susbcription for listen and reply
				MQTTListenerHandler(c, hubKey, configuration, communication)

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

func MQTTListenerHandler(mqttClient mqtt.Client, hubKey string, configuration *models.Configuration, communication *models.Communication) {
	if hubKey == "" {
		log.Log.Info("MQTTListenerHandler: no hub key provided, not subscribing to kerberos/hub/{hubkey}")
	} else {
		topicOnvif := fmt.Sprintf("kerberos/agent/%s", hubKey)
		mqttClient.Subscribe(topicOnvif, 1, func(c mqtt.Client, msg mqtt.Message) {

			// Decode the message, we are expecting following format.
			// {
			//   mid: string, "unique id for the message"
			//	 timestamp: int64, "unix timestamp when the message was generated"
			//   encrypted: boolean,
			//	 fingerprint: string, "fingerprint of the message to validate authenticity"
			//	 payload: Payload, "a json object which might be encrypted"
			// }

			var message Message
			json.Unmarshal(msg.Payload(), &message)
			if message.Mid != "" && message.Timestamp != 0 {
				// Messages might be encrypted, if so we'll
				// need to decrypt them.
				var payload Payload
				if message.Encrypted {
					// We'll find out the key we use to decrypt the message.
					// TODO -> still needs to be implemented.
					// Use to fingerprint to act accordingly.
				} else {
					payload = message.Payload
				}

				// We will receive all messages from our hub, so we'll need to filter to the relevant device.
				if payload.DeviceId != configuration.Config.Key {
					// Not relevant for this device, so we'll ignore it.
				} else {
					// We'll find out which message we received, and act accordingly.
					switch payload.Action {
					case "record":
						HandleRecording(mqttClient, hubKey, payload, configuration, communication)
					case "get-ptz-position":
						HandleGetPTZPosition(mqttClient, hubKey, payload, configuration, communication)
					case "update-ptz-position":
						HandleUpdatePTZPosition(mqttClient, hubKey, payload, configuration, communication)
					}
				}
			}
		})
	}
}

// We received a recording request, we'll send it to the motion handler.
type RecordPayload struct {
	Timestamp int64 `json:"timestamp"` // timestamp of the recording request.
}

func HandleRecording(mqttClient mqtt.Client, hubKey string, payload Payload, configuration *models.Configuration, communication *models.Communication) {
	value := payload.Value

	// Convert map[string]interface{} to RecordPayload
	jsonData, _ := json.Marshal(value)
	var recordPayload RecordPayload
	json.Unmarshal(jsonData, &recordPayload)

	if recordPayload.Timestamp != 0 {
		motionDataPartial := models.MotionDataPartial{
			Timestamp: recordPayload.Timestamp,
		}
		communication.HandleMotion <- motionDataPartial
	}
}

// We received a preset position request, we'll request it through onvif and send it back.
type PTZPositionPayload struct {
	Timestamp int64 `json:"timestamp"` // timestamp of the preset request.
}

func HandleGetPTZPosition(mqttClient mqtt.Client, hubKey string, payload Payload, configuration *models.Configuration, communication *models.Communication) {
	value := payload.Value

	// Convert map[string]interface{} to PTZPositionPayload
	jsonData, _ := json.Marshal(value)
	var positionPayload PTZPositionPayload
	json.Unmarshal(jsonData, &positionPayload)

	if positionPayload.Timestamp != 0 {
		// Get Position from device
		pos, err := onvif.GetPositionFromDevice(*configuration)
		if err != nil {
			log.Log.Error("HandlePTZPosition: error getting position from device: " + err.Error())
		} else {
			// Needs to wrapped!
			posString := fmt.Sprintf("%f,%f,%f", pos.PanTilt.X, pos.PanTilt.Y, pos.Zoom.X)
			message := Message{
				Payload: Payload{
					Action:   "ptz-position",
					DeviceId: configuration.Config.Key,
					Value: map[string]interface{}{
						"timestamp": positionPayload.Timestamp,
						"position":  posString,
					},
				},
			}
			payload, err := PackageMQTTMessage(message)
			if err == nil {
				mqttClient.Publish("kerberos/hub/"+hubKey, 0, false, payload)
			} else {
				log.Log.Info("HandlePTZPosition: something went wrong while sending position to hub: " + string(payload))
			}
		}
	}
}

func HandleUpdatePTZPosition(mqttClient mqtt.Client, hubKey string, payload Payload, configuration *models.Configuration, communication *models.Communication) {
	value := payload.Value

	// Convert map[string]interface{} to PTZPositionPayload
	jsonData, _ := json.Marshal(value)
	var onvifAction models.OnvifAction
	json.Unmarshal(jsonData, &onvifAction)

	if onvifAction.Action != "" {
		if communication.CameraConnected {
			communication.HandleONVIF <- onvifAction
			log.Log.Info("MQTTListenerHandleONVIF: Received an action - " + onvifAction.Action)
		} else {
			log.Log.Info("MQTTListenerHandleONVIF: received action, but camera is not connected.")
		}
	}
}

func DisconnectMQTT(mqttClient mqtt.Client, config *models.Config) {
	if mqttClient != nil {
		// Cleanup all subscriptions

		// New methods
		mqttClient.Unsubscribe("kerberos/agent/" + PREV_HubKey)

		// Legacy methods
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

// #################################################################################################
// Below you'll find legacy methods, as of now we'll have a single subscription, which scales better

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
