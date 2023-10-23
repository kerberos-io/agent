package mqtt

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"math/rand"
	"strconv"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	configService "github.com/kerberos-io/agent/machinery/src/config"
	"github.com/kerberos-io/agent/machinery/src/encryption"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/onvif"
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

// Configuring MQTT to subscribe for various bi-directional messaging
// Listen and reply (a generic method to share and retrieve information)
//
// - [SUBSCRIPTION] kerberos/agent/{hubkey} 		(hub -> agent)
// - [PUBLISH] kerberos/hub/{hubkey}  		(agent -> hub)
//
// !!! LEGACY METHODS BELOW, WE SHOULD LEVERAGE THE ABOVE METHOD!
// [PUBlISH]
// Next to subscribing to various topics, we'll also publish messages to various topics, find a list of available Publish methods.
// - kerberos/{hubkey}/device/{devicekey}/motion: a motion signal

func ConfigureMQTT(configDirectory string, configuration *models.Configuration, communication *models.Communication) mqtt.Client {

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
				MQTTListenerHandler(c, hubKey, configDirectory, configuration, communication)
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

func MQTTListenerHandler(mqttClient mqtt.Client, hubKey string, configDirectory string, configuration *models.Configuration, communication *models.Communication) {
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

			var message models.Message
			json.Unmarshal(msg.Payload(), &message)

			// We will receive all messages from our hub, so we'll need to filter to the relevant device.
			if message.Mid != "" && message.Timestamp != 0 && message.DeviceId == configuration.Config.Key {
				// Messages might be encrypted, if so we'll
				// need to decrypt them.
				var payload models.Payload
				if message.Encrypted && configuration.Config.Encryption != nil && configuration.Config.Encryption.Enabled == "true" {
					encryptedValue := message.Payload.EncryptedValue
					if len(encryptedValue) > 0 {
						symmetricKey := configuration.Config.Encryption.SymmetricKey
						privateKey := configuration.Config.Encryption.PrivateKey
						r := strings.NewReader(privateKey)
						pemBytes, _ := ioutil.ReadAll(r)
						block, _ := pem.Decode(pemBytes)
						if block == nil {
							log.Log.Error("MQTTListenerHandler: error decoding PEM block containing private key")
							return
						} else {
							// Parse private key
							b := block.Bytes
							key, err := x509.ParsePKCS8PrivateKey(b)
							if err != nil {
								log.Log.Error("MQTTListenerHandler: error parsing private key: " + err.Error())
								return
							} else {
								// Conver key to *rsa.PrivateKey
								rsaKey, _ := key.(*rsa.PrivateKey)

								// Get encrypted key from message, delimited by :::
								encryptedKey := strings.Split(encryptedValue, ":::")[0]   // encrypted with RSA
								encryptedValue := strings.Split(encryptedValue, ":::")[1] // encrypted with AES
								// Convert encrypted value to []byte
								decryptedKey, err := encryption.DecryptWithPrivateKey(encryptedKey, rsaKey)
								if decryptedKey != nil {
									if string(decryptedKey) == symmetricKey {
										// Decrypt value with decryptedKey
										data, err := base64.StdEncoding.DecodeString(encryptedValue)
										if err != nil {
											return
										}
										decryptedValue, err := encryption.AesDecrypt(data, string(decryptedKey))
										if err != nil {
											log.Log.Error("MQTTListenerHandler: error decrypting message: " + err.Error())
											return
										}
										json.Unmarshal(decryptedValue, &payload)
									} else {
										log.Log.Error("MQTTListenerHandler: error decrypting message, assymetric keys do not match.")
										return
									}
								} else if err != nil {
									log.Log.Error("MQTTListenerHandler: error decrypting message: " + err.Error())
									return
								}
							}
						}
					}
				} else {
					payload = message.Payload
				}

				// We'll find out which message we received, and act accordingly.
				log.Log.Info("MQTTListenerHandler: received message with action: " + payload.Action)
				switch payload.Action {
				case "record":
					go HandleRecording(mqttClient, hubKey, payload, configuration, communication)
				case "get-ptz-position":
					go HandleGetPTZPosition(mqttClient, hubKey, payload, configuration, communication)
				case "update-ptz-position":
					go HandleUpdatePTZPosition(mqttClient, hubKey, payload, configuration, communication)
				case "navigate-ptz":
					go HandleNavigatePTZ(mqttClient, hubKey, payload, configuration, communication)
				case "request-config":
					go HandleRequestConfig(mqttClient, hubKey, payload, configuration, communication)
				case "update-config":
					go HandleUpdateConfig(mqttClient, hubKey, payload, configDirectory, configuration, communication)
				case "request-sd-stream":
					go HandleRequestSDStream(mqttClient, hubKey, payload, configuration, communication)
				case "request-hd-stream":
					go HandleRequestHDStream(mqttClient, hubKey, payload, configuration, communication)
				case "receive-hd-candidates":
					go HandleReceiveHDCandidates(mqttClient, hubKey, payload, configuration, communication)
				}

			}
		})
	}
}

func HandleRecording(mqttClient mqtt.Client, hubKey string, payload models.Payload, configuration *models.Configuration, communication *models.Communication) {
	value := payload.Value

	// Convert map[string]interface{} to RecordPayload
	jsonData, _ := json.Marshal(value)
	var recordPayload models.RecordPayload
	json.Unmarshal(jsonData, &recordPayload)

	if recordPayload.Timestamp != 0 {
		motionDataPartial := models.MotionDataPartial{
			Timestamp: recordPayload.Timestamp,
		}
		communication.HandleMotion <- motionDataPartial
	}
}

func HandleGetPTZPosition(mqttClient mqtt.Client, hubKey string, payload models.Payload, configuration *models.Configuration, communication *models.Communication) {
	value := payload.Value

	// Convert map[string]interface{} to PTZPositionPayload
	jsonData, _ := json.Marshal(value)
	var positionPayload models.PTZPositionPayload
	json.Unmarshal(jsonData, &positionPayload)

	if positionPayload.Timestamp != 0 {
		// Get Position from device
		pos, err := onvif.GetPositionFromDevice(*configuration)
		if err != nil {
			log.Log.Error("HandlePTZPosition: error getting position from device: " + err.Error())
		} else {
			// Needs to wrapped!
			posString := fmt.Sprintf("%f,%f,%f", pos.PanTilt.X, pos.PanTilt.Y, pos.Zoom.X)
			message := models.Message{
				Payload: models.Payload{
					Action:   "ptz-position",
					DeviceId: configuration.Config.Key,
					Value: map[string]interface{}{
						"timestamp": positionPayload.Timestamp,
						"position":  posString,
					},
				},
			}
			payload, err := models.PackageMQTTMessage(configuration, message)
			if err == nil {
				mqttClient.Publish("kerberos/hub/"+hubKey, 0, false, payload)
			} else {
				log.Log.Info("HandlePTZPosition: something went wrong while sending position to hub: " + string(payload))
			}
		}
	}
}

func HandleUpdatePTZPosition(mqttClient mqtt.Client, hubKey string, payload models.Payload, configuration *models.Configuration, communication *models.Communication) {
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

func HandleRequestConfig(mqttClient mqtt.Client, hubKey string, payload models.Payload, configuration *models.Configuration, communication *models.Communication) {
	value := payload.Value

	// Convert map[string]interface{} to RequestConfigPayload
	jsonData, _ := json.Marshal(value)
	var configPayload models.RequestConfigPayload
	json.Unmarshal(jsonData, &configPayload)

	if configPayload.Timestamp != 0 {
		// Get Config from the device

		key := configuration.Config.Key
		name := configuration.Config.Name

		if key != "" && name != "" {

			// Copy the config, as we don't want to share the encryption part.
			deepCopy := configuration.Config

			var configMap map[string]interface{}
			inrec, _ := json.Marshal(deepCopy)
			json.Unmarshal(inrec, &configMap)

			// Unset encryption part.
			delete(configMap, "encryption")

			message := models.Message{
				Payload: models.Payload{
					Action:   "receive-config",
					DeviceId: configuration.Config.Key,
					Value:    configMap,
				},
			}
			payload, err := models.PackageMQTTMessage(configuration, message)
			if err == nil {
				mqttClient.Publish("kerberos/hub/"+hubKey, 0, false, payload)
			} else {
				log.Log.Info("HandleRequestConfig: something went wrong while sending config to hub: " + string(payload))
			}

		} else {
			log.Log.Info("HandleRequestConfig: no config available")
		}

		log.Log.Info("HandleRequestConfig: Received a request for the config")
	}
}

func HandleUpdateConfig(mqttClient mqtt.Client, hubKey string, payload models.Payload, configDirectory string, configuration *models.Configuration, communication *models.Communication) {
	value := payload.Value

	// Convert map[string]interface{} to UpdateConfigPayload
	jsonData, _ := json.Marshal(value)
	var configPayload models.UpdateConfigPayload
	json.Unmarshal(jsonData, &configPayload)

	if configPayload.Timestamp != 0 {

		config := configPayload.Config

		// Make sure to remove Encryption part, as we don't want to save it.
		config.Encryption = configuration.Config.Encryption

		err := configService.SaveConfig(configDirectory, config, configuration, communication)
		if err == nil {
			log.Log.Info("HandleUpdateConfig: Config updated")
			message := models.Message{
				Payload: models.Payload{
					Action:   "acknowledge-update-config",
					DeviceId: configuration.Config.Key,
				},
			}
			payload, err := models.PackageMQTTMessage(configuration, message)
			if err == nil {
				mqttClient.Publish("kerberos/hub/"+hubKey, 0, false, payload)
			} else {
				log.Log.Info("HandleRequestConfig: something went wrong while sending acknowledge config to hub: " + string(payload))
			}
		} else {
			log.Log.Info("HandleUpdateConfig: Config update failed")
		}
	}
}

func HandleRequestSDStream(mqttClient mqtt.Client, hubKey string, payload models.Payload, configuration *models.Configuration, communication *models.Communication) {
	value := payload.Value
	// Convert map[string]interface{} to RequestSDStreamPayload
	jsonData, _ := json.Marshal(value)
	var requestSDStreamPayload models.RequestSDStreamPayload
	json.Unmarshal(jsonData, &requestSDStreamPayload)

	if requestSDStreamPayload.Timestamp != 0 {
		if communication.CameraConnected {
			select {
			case communication.HandleLiveSD <- time.Now().Unix():
			default:
			}
			log.Log.Info("HandleRequestSDStream: received request to livestream.")
		} else {
			log.Log.Info("HandleRequestSDStream: received request to livestream, but camera is not connected.")
		}
	}
}

func HandleRequestHDStream(mqttClient mqtt.Client, hubKey string, payload models.Payload, configuration *models.Configuration, communication *models.Communication) {
	value := payload.Value
	// Convert map[string]interface{} to RequestHDStreamPayload
	jsonData, _ := json.Marshal(value)
	var requestHDStreamPayload models.RequestHDStreamPayload
	json.Unmarshal(jsonData, &requestHDStreamPayload)

	if requestHDStreamPayload.Timestamp != 0 {
		if communication.CameraConnected {
			// Set the Hub key, so we can send back the answer.
			requestHDStreamPayload.HubKey = hubKey
			select {
			case communication.HandleLiveHDHandshake <- requestHDStreamPayload:
			default:
			}
			log.Log.Info("HandleRequestHDStream: received request to setup webrtc.")
		} else {
			log.Log.Info("HandleRequestHDStream: received request to setup webrtc, but camera is not connected.")
		}
	}
}

func HandleReceiveHDCandidates(mqttClient mqtt.Client, hubKey string, payload models.Payload, configuration *models.Configuration, communication *models.Communication) {
	value := payload.Value
	// Convert map[string]interface{} to ReceiveHDCandidatesPayload
	jsonData, _ := json.Marshal(value)
	var receiveHDCandidatesPayload models.ReceiveHDCandidatesPayload
	json.Unmarshal(jsonData, &receiveHDCandidatesPayload)

	if receiveHDCandidatesPayload.Timestamp != 0 {
		if communication.CameraConnected {
			channel := webrtc.CandidateArrays[receiveHDCandidatesPayload.SessionID]
			log.Log.Info("HandleReceiveHDCandidates: " + receiveHDCandidatesPayload.Candidate)
			channel <- receiveHDCandidatesPayload.Candidate
		} else {
			log.Log.Info("HandleReceiveHDCandidates: received candidate, but camera is not connected.")
		}
	}
}

func HandleNavigatePTZ(mqttClient mqtt.Client, hubKey string, payload models.Payload, configuration *models.Configuration, communication *models.Communication) {
	value := payload.Value
	jsonData, _ := json.Marshal(value)
	var navigatePTZPayload models.NavigatePTZPayload
	json.Unmarshal(jsonData, &navigatePTZPayload)

	if navigatePTZPayload.Timestamp != 0 {
		if communication.CameraConnected {
			action := navigatePTZPayload.Action
			var onvifAction models.OnvifAction
			json.Unmarshal([]byte(action), &onvifAction)
			communication.HandleONVIF <- onvifAction
			log.Log.Info("HandleNavigatePTZ: Received an action - " + onvifAction.Action)

		} else {
			log.Log.Info("HandleNavigatePTZ: received action, but camera is not connected.")
		}
	}
}

func DisconnectMQTT(mqttClient mqtt.Client, config *models.Config) {
	if mqttClient != nil {
		// Cleanup all subscriptions
		// New methods
		mqttClient.Unsubscribe("kerberos/agent/" + PREV_HubKey)
		mqttClient.Disconnect(1000)
		mqttClient = nil
		log.Log.Info("DisconnectMQTT: MQTT client disconnected.")
	}
}
