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
	"sync"
	"time"

	"context"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/kerberos-io/agent/machinery/src/capture"
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
		log.Log.Info("routers.mqtt.main.ConfigureMQTT(): not starting as running in Offline mode.")
	} else {

		opts := mqtt.NewClientOptions()

		// We will set the MQTT endpoint to which we want to connect
		// and share and receive messages to/from.
		mqttURL := config.MQTTURI
		opts.AddBroker(mqttURL)
		log.Log.Debug("routers.mqtt.main.ConfigureMQTT(): Set broker uri " + mqttURL)

		// Our MQTT broker can have username/password credentials
		// to protect it from the outside.
		mqtt_username := config.MQTTUsername
		mqtt_password := config.MQTTPassword
		if mqtt_username != "" || mqtt_password != "" {
			opts.SetUsername(mqtt_username)
			opts.SetPassword(mqtt_password)
			log.Log.Debug("routers.mqtt.main.ConfigureMQTT(): Set username " + mqtt_username)
			log.Log.Debug("routers.mqtt.main.ConfigureMQTT(): Set password " + mqtt_password)
		}

		// Some extra options to make sure the connection behaves
		// properly. More information here: github.com/eclipse/paho.mqtt.golang.
		//opts.SetCleanSession(true)
		opts.SetCleanSession(false)
		opts.SetResumeSubs(true)
		opts.SetStore(mqtt.NewMemoryStore())
		opts.SetConnectRetry(true)
		opts.SetAutoReconnect(true)
		opts.SetConnectRetryInterval(5 * time.Second)
		opts.SetMaxReconnectInterval(1 * time.Minute)
		opts.SetKeepAlive(30 * time.Second)
		opts.SetPingTimeout(10 * time.Second)
		opts.SetWriteTimeout(10 * time.Second)
		opts.SetOrderMatters(false)
		opts.SetConnectTimeout(30 * time.Second)
		opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
			if err != nil {
				log.Log.Error("routers.mqtt.main.ConfigureMQTT(): MQTT connection lost: " + err.Error())
			} else {
				log.Log.Error("routers.mqtt.main.ConfigureMQTT(): MQTT connection lost")
			}
		})
		opts.SetReconnectingHandler(func(client mqtt.Client, options *mqtt.ClientOptions) {
			log.Log.Warning("routers.mqtt.main.ConfigureMQTT(): reconnecting to MQTT broker")
		})
		opts.SetOnConnectHandler(func(c mqtt.Client) {
			log.Log.Info("routers.mqtt.main.ConfigureMQTT(): MQTT session is online")
		})

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
			log.Log.Info("routers.mqtt.main.ConfigureMQTT(): Set ClientID " + mqttClientID)
			rand.Seed(time.Now().UnixNano())

			opts.OnConnect = func(c mqtt.Client) {
				// We managed to connect to the MQTT broker, hurray!
				log.Log.Info("routers.mqtt.main.ConfigureMQTT(): " + mqttClientID + " connected to " + mqttURL)

				// Create a susbcription for listen and reply
				MQTTListenerHandler(c, hubKey, configDirectory, configuration, communication)
			}
		}
		mqc := mqtt.NewClient(opts)
		if token := mqc.Connect(); token.WaitTimeout(30 * time.Second) {
			if token.Error() != nil {
				log.Log.Error("routers.mqtt.main.ConfigureMQTT(): unable to establish mqtt broker connection, error was: " + token.Error().Error())
			} else {
				log.Log.Info("routers.mqtt.main.ConfigureMQTT(): initial MQTT connection established")
			}
		} else {
			log.Log.Error("routers.mqtt.main.ConfigureMQTT(): timed out while establishing mqtt broker connection")
		}
		return mqc
	}

	return nil
}

// recentHDSessions tracks recently-seen WebRTC viewer session IDs so we can
// dedupe duplicate request-hd-stream messages without relying on the broker's
// (and the viewer's) wall clock. The viewer's offer-republish loop can fire
// the same request several times for the same session_id while waiting for an
// answer; the broker can also redeliver a message after a reconnect with
// CleanSession=false. In both cases we want to handle the session exactly
// once.
//
// Entries expire after recentHDSessionTTL. The map is small (one entry per
// active viewer over the TTL window) so a periodic sweep is sufficient.
const recentHDSessionTTL = 60 * time.Second

var (
	recentHDSessionsMu sync.Mutex
	recentHDSessions   = make(map[string]time.Time)
)

// markHDSessionSeen returns true if this session_id was already processed
// within the TTL window (i.e. this message should be treated as a duplicate).
// It also opportunistically prunes expired entries.
func markHDSessionSeen(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	recentHDSessionsMu.Lock()
	defer recentHDSessionsMu.Unlock()
	now := time.Now()
	// Lazy GC — cheap given the expected map size.
	for k, t := range recentHDSessions {
		if now.Sub(t) > recentHDSessionTTL {
			delete(recentHDSessions, k)
		}
	}
	if _, exists := recentHDSessions[sessionID]; exists {
		return true
	}
	recentHDSessions[sessionID] = now
	return false
}

func MQTTListenerHandler(mqttClient mqtt.Client, hubKey string, configDirectory string, configuration *models.Configuration, communication *models.Communication) {
	if hubKey == "" {
		log.Log.Info("routers.mqtt.main.MQTTListenerHandler(): no hub key provided, not subscribing to kerberos/hub/{hubkey}")
	} else {
		agentListener := fmt.Sprintf("kerberos/agent/%s", hubKey)
		token := mqttClient.Subscribe(agentListener, 1, func(c mqtt.Client, msg mqtt.Message) {

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
				var payload models.Payload

				// Messages might be hidden, if so we'll need to decrypt them using the Kerberos Hub private key.
				if message.Hidden && configuration.Config.HubEncryption == "true" {
					hiddenValue := message.Payload.HiddenValue
					if len(hiddenValue) > 0 {
						privateKey := configuration.Config.HubPrivateKey
						if privateKey != "" {
							data, err := base64.StdEncoding.DecodeString(hiddenValue)
							if err != nil {
								return
							}
							visibleValue, err := encryption.AesDecrypt(data, privateKey)
							if err != nil {
								log.Log.Error("routers.mqtt.main.MQTTListenerHandler(): error decrypting message: " + err.Error())
								return
							}
							json.Unmarshal(visibleValue, &payload)
							message.Payload = payload
						} else {
							log.Log.Error("routers.mqtt.main.MQTTListenerHandler(): error decrypting message, no private key provided.")
						}
					}
				}

				// Messages might be end-to-end encrypted, if so we'll need to decrypt them,
				// using our own keys.
				if message.Encrypted && configuration.Config.Encryption != nil && configuration.Config.Encryption.Enabled == "true" {
					encryptedValue := message.Payload.EncryptedValue
					if len(encryptedValue) > 0 {
						symmetricKey := configuration.Config.Encryption.SymmetricKey
						privateKey := configuration.Config.Encryption.PrivateKey
						r := strings.NewReader(privateKey)
						pemBytes, _ := ioutil.ReadAll(r)
						block, _ := pem.Decode(pemBytes)
						if block == nil {
							log.Log.Error("routers.mqtt.main.MQTTListenerHandler(): error decoding PEM block containing private key")
							return
						} else {
							// Parse private key
							b := block.Bytes
							key, err := x509.ParsePKCS8PrivateKey(b)
							if err != nil {
								log.Log.Error("routers.mqtt.main.MQTTListenerHandler(): error parsing private key: " + err.Error())
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
											log.Log.Error("routers.mqtt.main.MQTTListenerHandler(): error decrypting message: " + err.Error())
											return
										}
										json.Unmarshal(decryptedValue, &payload)
									} else {
										log.Log.Error("routers.mqtt.main.MQTTListenerHandler(): error decrypting message, assymetric keys do not match.")
										return
									}
								} else if err != nil {
									log.Log.Error("routers.mqtt.main.MQTTListenerHandler(): error decrypting message: " + err.Error())
									return
								}
							}
						}
					}
				} else {
					payload = message.Payload
				}

				// We'll find out which message we received, and act accordingly.
				log.Log.Info("routers.mqtt.main.MQTTListenerHandler(): received message with action: " + payload.Action)

				// NOTE: We intentionally do NOT discard request-hd-stream /
				// receive-hd-candidates messages based on a wall-clock age. The
				// viewer and agent clocks can drift (especially on embedded
				// devices), which previously caused valid requests to be
				// silently dropped and forced the user to refresh the page.
				// Duplicate handling for request-hd-stream is done by session_id
				// inside HandleRequestHDStream (see markHDSessionSeen).

				switch payload.Action {
				case "record":
					go HandleRecording(mqttClient, hubKey, payload, configuration, communication)
				case "get-audio-backchannel":
					go HandleAudio(mqttClient, hubKey, payload, configuration, communication)
				case "get-ptz-position":
					go HandleGetPTZPosition(mqttClient, hubKey, payload, configuration, communication)
				case "update-ptz-position":
					go HandleUpdatePTZPosition(mqttClient, hubKey, payload, configuration, communication)
				case "navigate-ptz":
					go HandleNavigatePTZ(mqttClient, hubKey, payload, configuration, communication)
				case "request-config":
					go HandleRequestConfig(mqttClient, hubKey, payload, configuration, communication)
				case "verify-stream":
					go HandleVerifyStream(mqttClient, hubKey, payload, configuration, communication)
				case "update-config":
					go HandleUpdateConfig(mqttClient, hubKey, payload, configDirectory, configuration, communication)
				case "request-sd-stream":
					go HandleRequestSDStream(mqttClient, hubKey, payload, configuration, communication)
				case "request-hd-stream":
					go HandleRequestHDStream(mqttClient, hubKey, payload, configuration, communication)
				case "request-hls-stream":
					go HandleRequestHLSStream(mqttClient, hubKey, payload, configuration, communication)
				case "receive-hd-candidates":
					go HandleReceiveHDCandidates(mqttClient, hubKey, payload, configuration, communication)
				case "trigger-relay":
					go HandleTriggerRelay(mqttClient, hubKey, payload, configuration, communication)
				}

			}
		})

		if token.WaitTimeout(10 * time.Second) {
			if token.Error() != nil {
				log.Log.Error("routers.mqtt.main.MQTTListenerHandler(): failed to subscribe to " + agentListener + ": " + token.Error().Error())
			} else {
				log.Log.Info("routers.mqtt.main.MQTTListenerHandler(): subscribed to " + agentListener)
			}
		} else {
			log.Log.Error("routers.mqtt.main.MQTTListenerHandler(): timed out while subscribing to " + agentListener)
		}
	}
}

func HandleRecording(mqttClient mqtt.Client, hubKey string, payload models.Payload, configuration *models.Configuration, communication *models.Communication) {
	value := payload.Value

	// Convert map[string]interface{} to RecordPayload
	jsonData, _ := json.Marshal(value)
	var recordPayload models.RecordPayload
	json.Unmarshal(jsonData, &recordPayload)

	timestamp := recordPayload.Timestamp
	if timestamp == 0 {
		timestamp = time.Now().Unix()
	}

	if recordPayload.Recording {
		now := time.Now().UnixMilli()
		if recordPayload.Heartbeat {
			// Keep-alive from a viewer that supports heartbeats. Only refresh while
			// a manual recording is actually running; if it already auto-stopped
			// (heartbeat timeout / max duration) we IGNORE it so a stray heartbeat
			// can't restart a recording we just ended. Seeing a heartbeat also arms
			// the recorder's heartbeat-timeout auto-stop.
			if communication.IsRecordingManual.IsSet() {
				communication.RecordingManualHeartbeat.Store(now)
				communication.RecordingManualHeartbeatSeen.Set()
				log.Log.Debug("routers.mqtt.main.HandleRecording(): manual recording heartbeat received.")
			} else {
				log.Log.Debug("routers.mqtt.main.HandleRecording(): ignoring heartbeat, no active manual recording.")
			}
		} else {
			// Explicit start from the live view (record button). Start a manual
			// recording and keep it running — the motion recorder honours
			// communication.IsRecordingManual and won't auto-close on the
			// post-recording timeout while it's set. We also inject a motion event
			// so the recording starts immediately, even when nothing is moving.
			communication.RecordingManualHeartbeat.Store(now)
			if communication.IsRecordingManual.SetToIf(false, true) {
				communication.RecordingManualStart.Store(now)
				communication.RecordingManualHeartbeatSeen.UnSet()
				log.Log.Info("routers.mqtt.main.HandleRecording(): manual recording started.")
				select {
				case communication.HandleMotion <- models.MotionDataPartial{Timestamp: timestamp, NumberOfChanges: 100000000}:
				default:
					log.Log.Warning("routers.mqtt.main.HandleRecording(): motion channel full, manual recording start not queued.")
				}
			}
		}
	} else {
		// Stop the manual recording; the motion recorder closes the clip once the
		// post-recording window elapses. Clear the heartbeat/start markers too.
		log.Log.Info("routers.mqtt.main.HandleRecording(): manual recording stopped.")
		communication.IsRecordingManual.UnSet()
		communication.RecordingManualHeartbeat.Store(0)
		communication.RecordingManualStart.Store(0)
		communication.RecordingManualHeartbeatSeen.UnSet()
	}
}

func HandleAudio(mqttClient mqtt.Client, hubKey string, payload models.Payload, configuration *models.Configuration, communication *models.Communication) {
	value := payload.Value

	// Convert map[string]interface{} to AudioPayload
	jsonData, _ := json.Marshal(value)
	var audioPayload models.AudioPayload
	json.Unmarshal(jsonData, &audioPayload)

	if audioPayload.Timestamp != 0 {
		audioDataPartial := models.AudioDataPartial{
			Timestamp: audioPayload.Timestamp,
			Data:      audioPayload.Data,
		}
		communication.HandleAudio <- audioDataPartial
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
			log.Log.Error("routers.mqtt.main.HandlePTZPosition(): error getting position from device: " + err.Error())
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
				mqttClient.Publish("kerberos/hub/"+hubKey, 2, false, payload)
			} else {
				log.Log.Info("routers.mqtt.main.HandlePTZPosition(): something went wrong while sending position to hub: " + string(payload))
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
			log.Log.Info("routers.mqtt.main.MQTTListenerHandleONVIF(): Received an action - " + onvifAction.Action)
		} else {
			log.Log.Info("routers.mqtt.main.MQTTListenerHandleONVIF(): received action, but camera is not connected.")
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
		if configuration.Config.FriendlyName != "" {
			name = configuration.Config.FriendlyName
		}

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
				mqttClient.Publish("kerberos/hub/"+hubKey, 2, false, payload)
			} else {
				log.Log.Info("routers.mqtt.main.HandleRequestConfig(): something went wrong while sending config to hub: " + string(payload))
			}

		} else {
			log.Log.Info("routers.mqtt.main.HandleRequestConfig(): no config available")
		}

		log.Log.Info("routers.mqtt.main.HandleRequestConfig(): Received a request for the config")
	}
}

// HandleVerifyStream probes an RTSP stream (the one supplied in the request, or
// the currently configured main/sub stream) and reports back whether it can be
// connected to and decoded, along with the discovered codec/resolution/fps.
func HandleVerifyStream(mqttClient mqtt.Client, hubKey string, payload models.Payload, configuration *models.Configuration, communication *models.Communication) {
	value := payload.Value

	// Convert map[string]interface{} to VerifyStreamPayload
	jsonData, _ := json.Marshal(value)
	var verifyPayload models.VerifyStreamPayload
	json.Unmarshal(jsonData, &verifyPayload)

	if verifyPayload.Timestamp == 0 {
		return
	}

	stream := verifyPayload.Stream
	if stream != "sub" {
		stream = "main"
	}

	// Resolve which RTSP url to verify: prefer the one supplied in the request
	// (so users can verify unsaved edits), otherwise fall back to the configured
	// stream url for the requested stream type.
	rtspUrl := verifyPayload.RTSP
	if rtspUrl == "" {
		if stream == "sub" {
			rtspUrl = configuration.Config.Capture.IPCamera.SubRTSP
		} else {
			rtspUrl = configuration.Config.Capture.IPCamera.RTSP
		}
	}

	success := false
	errMsg := ""
	width := 0
	height := 0
	codec := ""
	fps := 0.0

	if rtspUrl == "" {
		errMsg = "No RTSP url configured for this stream."
	} else {
		// Probe the stream with a bounded timeout so a dead/unreachable camera
		// can't hang the handler goroutine.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		rtspClient := &capture.Golibrtsp{Url: rtspUrl}
		errConnect := rtspClient.Connect(ctx, ctx)
		if errConnect != nil {
			errMsg = errConnect.Error()
		} else {
			videoStreams, errStreams := rtspClient.GetVideoStreams()
			if errStreams != nil || len(videoStreams) == 0 {
				errMsg = "Connected, but no decodable video stream was found."
			} else {
				success = true
				vs := videoStreams[0]
				width = vs.Width
				height = vs.Height
				codec = vs.Name
				fps = vs.FPS
			}
		}
		// Always release the connection.
		rtspClient.Close(ctx)
	}

	message := models.Message{
		Payload: models.Payload{
			Action:   "verify-stream-result",
			DeviceId: configuration.Config.Key,
			Value: map[string]interface{}{
				"timestamp": verifyPayload.Timestamp,
				"stream":    stream,
				"success":   success,
				"error":     errMsg,
				"width":     width,
				"height":    height,
				"codec":     codec,
				"fps":       fps,
			},
		},
	}
	packagedPayload, err := models.PackageMQTTMessage(configuration, message)
	if err == nil {
		mqttClient.Publish("kerberos/hub/"+hubKey, 2, false, packagedPayload)
	} else {
		log.Log.Info("routers.mqtt.main.HandleVerifyStream(): something went wrong while sending result to hub: " + string(packagedPayload))
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
			log.Log.Info("routers.mqtt.main.HandleUpdateConfig(): Config updated")
			message := models.Message{
				Payload: models.Payload{
					Action:   "acknowledge-update-config",
					DeviceId: configuration.Config.Key,
				},
			}
			payload, err := models.PackageMQTTMessage(configuration, message)
			if err == nil {
				mqttClient.Publish("kerberos/hub/"+hubKey, 2, false, payload)
			} else {
				log.Log.Info("routers.mqtt.main.HandleUpdateConfig(): something went wrong while sending acknowledge config to hub: " + string(payload))
			}
		} else {
			log.Log.Info("routers.mqtt.main.HandleUpdateConfig(): Config update failed")
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
			// A viewer that opted into the HTTP transport is signalled on a separate
			// channel so the producer ships its frames to hub-api over HTTP instead of
			// publishing them over MQTT. Any other (or absent) transport keeps the
			// legacy MQTT image push, so older frontends behave exactly as before.
			if requestSDStreamPayload.Transport == "http" {
				select {
				case communication.HandleLiveSDHTTP <- time.Now().Unix():
				default:
				}
			} else {
				select {
				case communication.HandleLiveSD <- time.Now().Unix():
				default:
				}
			}
			log.Log.Info("routers.mqtt.main.HandleRequestSDStream(): received request to livestream.")
		} else {
			log.Log.Info("routers.mqtt.main.HandleRequestSDStream(): received request to livestream, but camera is not connected.")
		}
	}
}

// HandleRequestHLSStream is the viewer keepalive for live HLS. Like the SD
// stream it simply signals that a viewer is watching; the agent owns the live
// HLS session, so a single non-zero timestamp on the channel keeps the segment
// pipeline alive (see cloud.HandleLiveStreamHLS). Viewers republish this
// periodically; when the keepalives stop, the agent tears the session down.
func HandleRequestHLSStream(mqttClient mqtt.Client, hubKey string, payload models.Payload, configuration *models.Configuration, communication *models.Communication) {
	value := payload.Value
	jsonData, _ := json.Marshal(value)
	var requestHLSStreamPayload models.RequestHLSStreamPayload
	json.Unmarshal(jsonData, &requestHLSStreamPayload)

	if requestHLSStreamPayload.Timestamp != 0 {
		if communication.CameraConnected {
			// Forward the requested quality ("auto"|"high"|"low"; empty => auto) so
			// the producer can switch the live session between the main and sub
			// stream on demand. The send doubles as the viewer keepalive.
			select {
			case communication.HandleLiveHLS <- requestHLSStreamPayload.Quality:
			default:
			}
			log.Log.Info("routers.mqtt.main.HandleRequestHLSStream(): received request to livestream over HLS.")
		} else {
			log.Log.Info("routers.mqtt.main.HandleRequestHLSStream(): received request to livestream over HLS, but camera is not connected.")
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
			// Dedupe by session_id: the viewer republishes its offer while
			// waiting for an answer (and the broker may redeliver), and we
			// don't want to spawn multiple peer connections for the same
			// browser session.
			if markHDSessionSeen(requestHDStreamPayload.SessionID) {
				log.Log.Info("routers.mqtt.main.HandleRequestHDStream(): duplicate request for session " +
					requestHDStreamPayload.SessionID + ", ignoring")
				return
			}
			// Set the Hub key, so we can send back the answer.
			requestHDStreamPayload.HubKey = hubKey
			if communication.HandleLiveHDHandshake == nil {
				log.Log.Error("routers.mqtt.main.HandleRequestHDStream(): handshake channel is nil, dropping request")
				return
			}

			communication.HandleLiveHDHandshake <- models.LiveHDHandshake{
				Payload: requestHDStreamPayload,
			}
			log.Log.Info("routers.mqtt.main.HandleRequestHDStream(): received request to setup webrtc.")
		} else {
			log.Log.Info("routers.mqtt.main.HandleRequestHDStream(): received request to setup webrtc, but camera is not connected.")
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
			// Register candidate channel
			key := configuration.Config.Key + "/" + receiveHDCandidatesPayload.SessionID
			go webrtc.RegisterCandidates(key, receiveHDCandidatesPayload)
		} else {
			log.Log.Info("routers.mqtt.main.HandleReceiveHDCandidates(): received candidate, but camera is not connected.")
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
			log.Log.Info("routers.mqtt.main.HandleNavigatePTZ(): Received an action - " + onvifAction.Action)
		} else {
			log.Log.Info("routers.mqtt.main.HandleNavigatePTZ(): received action, but camera is not connected.")
		}
	}
}

func HandleTriggerRelay(mqttClient mqtt.Client, hubKey string, payload models.Payload, configuration *models.Configuration, communication *models.Communication) {
	value := payload.Value
	jsonData, _ := json.Marshal(value)
	var triggerRelayPayload models.TriggerRelay
	json.Unmarshal(jsonData, &triggerRelayPayload)

	if triggerRelayPayload.Timestamp != 0 {
		if communication.CameraConnected {
			// Get token (name of relay)
			token := triggerRelayPayload.Token
			// Connect to Onvif device
			cameraConfiguration := configuration.Config.Capture.IPCamera
			device, _, err := onvif.ConnectToOnvifDevice(&cameraConfiguration)
			if err == nil {
				// Trigger relay output
				err := onvif.TriggerRelayOutput(device, token)
				if err != nil {
					log.Log.Error("routers.mqtt.main.HandleTriggerRelay(): error triggering relay: " + err.Error())
				} else {
					log.Log.Info("routers.mqtt.main.HandleTriggerRelay(): trigger (" + token + ") relay output.")
				}
			} else {
				log.Log.Error("routers.mqtt.main.HandleTriggerRelay(): error connecting to device: " + err.Error())
			}

		} else {
			log.Log.Info("routers.mqtt.main.HandleTriggerRelay(): received trigger, but camera is not connected.")
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
		log.Log.Info("routers.mqtt.main.DisconnectMQTT(): MQTT client disconnected.")
	}
}
