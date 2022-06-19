package components

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/kerberos-io/agent/machinery/src/models"
)

var (
	CandidateArrays map[string](chan string)
)

func ConfigureMQTT(log Logging,
	config *models.Config,
	livestreamChan chan int64,
	webrtcChan chan models.SDPPayload,
	webrtcKeepAlive chan string,
	webrtcPeers chan string,
	onvifActions chan models.OnvifAction,
) mqtt.Client {
	opts := mqtt.NewClientOptions()
	mqttURL := config.MQTTURI
	opts.AddBroker(mqttURL)
	mqtt_username := config.MQTTUsername
	mqtt_password := config.MQTTPassword
	if mqtt_username != "" || mqtt_password != "" {
		opts.SetUsername(mqtt_username)
		opts.SetPassword(mqtt_password)
		log.Info("MQTT: set username " + mqtt_username)
		log.Info("MQTT: set password " + mqtt_password)
	}

	opts.SetCleanSession(true)
	opts.SetConnectRetry(true)
	opts.SetConnectTimeout(30 * time.Second)

	key := ""
	// This is the old way ;)
	if config.Cloud == "s3" && config.S3.Publickey != "" {
		key = config.S3.Publickey
	} else if config.Cloud == "kstorage" && config.KStorage.CloudKey != "" {
		key = config.KStorage.CloudKey
	}
	// This is the new way ;)
	if config.HubKey != "" {
		key = config.HubKey
	}

	if key != "" {
		rand.Seed(time.Now().UnixNano())
		random := rand.Intn(100)
		randomKerberosHubKey := config.Key + strconv.Itoa(random) // this random int is to avoid conflicts.
		// This is a worked-around.
		// current S3 (Kerberos Hub SAAS) is using a secured MQTT, where the client id,
		// should match the kerberos hub key.
		if config.Cloud == "s3" {
			randomKerberosHubKey = config.Key
		}
		opts.SetClientID(randomKerberosHubKey)
		log.Info("MQTT: set ClientID " + randomKerberosHubKey)
		rand.Seed(time.Now().UnixNano())
		CandidateArrays = make(map[string](chan string))

		opts.OnConnect = func(c mqtt.Client) {

			// Create a subscription for the MQTT livestream.
			log.Info("MQTT " + randomKerberosHubKey + ": connected to " + mqttURL)
			topicRequest := "kerberos/" + key + "/device/" + config.Key + "/request-live"
			log.Info("MQTT: sending logs to " + topicRequest)
			c.Subscribe(topicRequest, 0, func(c mqtt.Client, msg mqtt.Message) {
				select {
				case livestreamChan <- time.Now().Unix():
				default:
				}
				log.Info("Livestream " + randomKerberosHubKey + ": received request to livestream.")
				msg.Ack()
			})

			// Create a subscription for the WEBRTC livestream.
			topicRequestWebRtc := config.Key + "/register"
			c.Subscribe(topicRequestWebRtc, 0, func(c mqtt.Client, msg mqtt.Message) {
				log.Info("Webrtc " + randomKerberosHubKey + ": received request to setup webrtc.")
				var sdp models.SDPPayload
				json.Unmarshal(msg.Payload(), &sdp)
				select {
				case webrtcChan <- sdp:
				default:
				}
				msg.Ack()
			})

			topicKeepAlive := fmt.Sprintf("kerberos/webrtc/keepalivehub/%s", config.Key)
			c.Subscribe(topicKeepAlive, 0, func(c mqtt.Client, msg mqtt.Message) {
				alive := string(msg.Payload())
				webrtcKeepAlive <- alive
				log.Info("WEBRTC (Forward): Received keepalive: " + alive)
			})

			topicPeers := fmt.Sprintf("kerberos/webrtc/peers/%s", config.Key)
			c.Subscribe(topicPeers, 0, func(c mqtt.Client, msg mqtt.Message) {
				peerCount := string(msg.Payload())
				webrtcPeers <- peerCount
				log.Info("WEBRTC (Forward): Number of peers listening: " + peerCount)
			})

			//candidates := make(chan string)
			// Listen for candidates
			c.Subscribe("candidate/cloud", 0, func(c mqtt.Client, msg mqtt.Message) {
				var candidate models.Candidate
				json.Unmarshal(msg.Payload(), &candidate)
				if candidate.CloudKey == config.Key {
					key := candidate.CloudKey + "/" + candidate.Cuuid
					channel, ok := CandidateArrays[key]
					if !ok {
						val := make(chan string, 30)
						CandidateArrays[key] = val
						channel = val
					}
					log.Info("WEBRTC (Remote): " + string(msg.Payload()))
					channel <- string(msg.Payload())
				}
			})

			// Receives ONVIF actions
			topicOnvif := fmt.Sprintf("kerberos/onvif/%s", config.Key)
			c.Subscribe(topicOnvif, 0, func(c mqtt.Client, msg mqtt.Message) {
				var onvifAction models.OnvifAction
				json.Unmarshal(msg.Payload(), &onvifAction)
				onvifActions <- onvifAction
				log.Info("Onvif (action): Received an action - " + onvifAction.Action)
			})
		}
	}
	mqc := mqtt.NewClient(opts)
	if token := mqc.Connect(); token.WaitTimeout(5 * time.Second) {
		if token.Error() != nil {
			log.Error("MQTT: unable to establish mqtt broker connection, error was: " + token.Error().Error())
		}
	}
	return mqc
}

func DisconnectMQTT(log Logging, config models.Config, mqc mqtt.Client) {
	//topicRequest := "kerberos/" + config.S3.Publickey + "/device/" + config.Key + "/request-live"
	//mqc.Unsubscribe(topicRequest)
	mqc.Disconnect(1000)
}
