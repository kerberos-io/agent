package models

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"strings"
	"time"

	"github.com/gofrs/uuid"
	"github.com/kerberos-io/agent/machinery/src/encryption"
	"github.com/kerberos-io/agent/machinery/src/log"
)

func PackageMQTTMessage(configuration *Configuration, msg Message) ([]byte, error) {
	// Create a Version 4 UUID.
	u2, err := uuid.NewV4()
	if err != nil {
		log.Log.Error("failed to generate UUID: " + err.Error())
	}

	// We'll generate an unique id, and encrypt / decrypt it using the private key if available.
	msg.Mid = u2.String()
	msg.DeviceId = msg.Payload.DeviceId
	msg.Timestamp = time.Now().Unix()

	// At the moment we don't do the encryption part, but we'll implement it
	// once the legacy methods (subscriptions are moved).
	msg.Encrypted = false
	if configuration.Config.Encryption != nil && configuration.Config.Encryption.Enabled == "true" {
		msg.Encrypted = true
	}
	msg.PublicKey = ""
	msg.Fingerprint = ""

	if msg.Encrypted {
		pload := msg.Payload

		// Pload to base64
		data, err := json.Marshal(pload)
		if err != nil {
			log.Log.Error("models.mqtt.PackageMQTTMessage(): failed to marshal payload: " + err.Error())
		}

		// Encrypt the value
		privateKey := configuration.Config.Encryption.PrivateKey
		r := strings.NewReader(privateKey)
		pemBytes, _ := io.ReadAll(r)
		block, _ := pem.Decode(pemBytes)
		if block == nil {
			log.Log.Error("models.mqtt.PackageMQTTMessage(): error decoding PEM block containing private key")
		} else {
			// Parse private key
			b := block.Bytes
			key, err := x509.ParsePKCS8PrivateKey(b)
			if err != nil {
				log.Log.Error("models.mqtt.PackageMQTTMessage(): error parsing private key: " + err.Error())
			}

			// Conver key to *rsa.PrivateKey
			rsaKey, _ := key.(*rsa.PrivateKey)

			// Create a 16bit key random
			k := configuration.Config.Encryption.SymmetricKey
			encryptedValue, err := encryption.AesEncrypt(data, k)
			if err == nil {

				data := base64.StdEncoding.EncodeToString(encryptedValue)
				// Sign the encrypted value
				signature, err := encryption.SignWithPrivateKey([]byte(data), rsaKey)
				if err == nil {
					base64Signature := base64.StdEncoding.EncodeToString(signature)
					msg.Payload.EncryptedValue = data
					msg.Payload.Signature = base64Signature
					msg.Payload.Value = make(map[string]interface{})
				}
			}
		}
	}

	payload, err := json.Marshal(msg)
	return payload, err
}

// The message structure which is used to send over
// and receive messages from the MQTT broker
type Message struct {
	Mid         string  `json:"mid"`
	DeviceId    string  `json:"device_id"`
	Timestamp   int64   `json:"timestamp"`
	Encrypted   bool    `json:"encrypted"`
	PublicKey   string  `json:"public_key"`
	Fingerprint string  `json:"fingerprint"`
	Payload     Payload `json:"payload"`
}

// The payload structure which is used to send over
// and receive messages from the MQTT broker
type Payload struct {
	Action         string                 `json:"action"`
	DeviceId       string                 `json:"device_id"`
	Signature      string                 `json:"signature"`
	EncryptedValue string                 `json:"encrypted_value"`
	Value          map[string]interface{} `json:"value"`
}

// We received a audio input
type AudioPayload struct {
	Timestamp int64   `json:"timestamp"` // timestamp of the recording request.
	Data      []int16 `json:"data"`
}

// We received a recording request, we'll send it to the motion handler.
type RecordPayload struct {
	Timestamp int64 `json:"timestamp"` // timestamp of the recording request.
}

// We received a preset position request, we'll request it through onvif and send it back.
type PTZPositionPayload struct {
	Timestamp int64 `json:"timestamp"` // timestamp of the preset request.
}

// We received a request config request, we'll fetch the current config and send it back.
type RequestConfigPayload struct {
	Timestamp int64 `json:"timestamp"` // timestamp of the preset request.
}

// We received a update config request, we'll update the current config and send a confirmation back.
type UpdateConfigPayload struct {
	Timestamp int64  `json:"timestamp"` // timestamp of the preset request.
	Config    Config `json:"config"`
}

// We received a request SD stream request
type RequestSDStreamPayload struct {
	Timestamp int64 `json:"timestamp"` // timestamp
}

// We received a request HD stream request
type RequestHDStreamPayload struct {
	Timestamp          int64  `json:"timestamp"`           // timestamp
	HubKey             string `json:"hub_key"`             // hub key
	SessionID          string `json:"session_id"`          // session id
	SessionDescription string `json:"session_description"` // session description
}

// We received a receive HD candidates request
type ReceiveHDCandidatesPayload struct {
	Timestamp int64  `json:"timestamp"`  // timestamp
	SessionID string `json:"session_id"` // session id
	Candidate string `json:"candidate"`  // candidate
}

type NavigatePTZPayload struct {
	Timestamp int64  `json:"timestamp"` // timestamp
	DeviceId  string `json:"device_id"` // device id
	Action    string `json:"action"`    // action
}
