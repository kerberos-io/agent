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

	// Configuration
	config := configuration.Config

	// Next to hiding the message, we can also encrypt it using your own private key.
	// Which is not stored in a remote environment (hence you are the only one owning it).
	msg.Encrypted = false
	if config.Encryption != nil && config.Encryption.Enabled == "true" {
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
			if config.Encryption != nil && config.Encryption.SymmetricKey != "" {
				k := config.Encryption.SymmetricKey
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
	}

	// We'll hide the message (by default in latest version)
	// We will encrypt using the Kerberos Hub private key if set.
	msg.Hidden = false
	if config.HubEncryption == "true" && config.HubPrivateKey != "" {
		msg.Hidden = true
	}

	if msg.Hidden {
		pload := msg.Payload
		// Pload to base64
		data, err := json.Marshal(pload)
		if err != nil {
			msg.Hidden = false
		} else {
			k := config.HubPrivateKey
			encryptedValue, err := encryption.AesEncrypt(data, k)
			if err == nil {
				data := base64.StdEncoding.EncodeToString(encryptedValue)
				msg.Payload.HiddenValue = data
				msg.Payload.EncryptedValue = ""
				msg.Payload.Signature = ""
				msg.Payload.Value = make(map[string]interface{})
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
	Hidden      bool    `json:"hidden"`
	PublicKey   string  `json:"public_key"`
	Fingerprint string  `json:"fingerprint"`
	Payload     Payload `json:"payload"`
}

// The payload structure which is used to send over
// and receive messages from the MQTT broker
type Payload struct {
	Version        string                 `json:"version"` // Version of the message, e.g. "1.0"
	Action         string                 `json:"action"`
	DeviceId       string                 `json:"device_id"`
	Signature      string                 `json:"signature"`
	EncryptedValue string                 `json:"encrypted_value"`
	HiddenValue    string                 `json:"hidden_value"`
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
	// Recording toggles a manual recording from the live view: true starts a
	// recording (and keeps it running), false stops it. Older clients that only
	// send a timestamp default to false; the live view always sets it explicitly.
	Recording bool `json:"recording"`
	// Heartbeat marks a keep-alive re-send (with Recording=true) from a viewer
	// that supports heartbeating, as opposed to the initial start (the record
	// button). While a user stays on the page the live view re-sends the record
	// command every few seconds; the agent uses this flag to (a) refresh the
	// recording's keep-alive without restarting an already auto-stopped clip from
	// a stray heartbeat, and (b) only enable the heartbeat-timeout auto-stop once
	// it has actually seen a heartbeat — so older viewers that never heartbeat
	// still record up to the max-duration cap instead of being cut off early.
	Heartbeat bool `json:"heartbeat"`
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
	// Transport selects how the agent should deliver the preview frames for this
	// viewer. "http" asks the agent to POST frames to hub-api (keeping them off
	// MQTT); empty/absent means the legacy MQTT image push. Older agents simply
	// ignore this unknown field and keep doing MQTT, and older frontends never set
	// it — so new/old agents and frontends interoperate in every combination.
	Transport string `json:"transport,omitempty"`
}

// Stream quality tiers a viewer can request for the live (HD) view. The agent
// maps these onto the camera's main (high-resolution) or sub (low-resolution)
// RTSP stream, so a viewer can pick the resolution it needs instead of the agent
// always preferring the sub stream. Empty/unknown values are treated as "auto"
// for backward compatibility: older frontends that never set a quality keep the
// previous behaviour (sub stream when available, otherwise main).
const (
	StreamQualityAuto = "auto" // agent decides based on availability/resolution
	StreamQualityHigh = "high" // main stream (highest resolution)
	StreamQualityLow  = "low"  // sub stream (lowest resolution)
)

// We received a live HLS stream request. Like SD it is a simple viewer
// keepalive: the agent owns the live HLS session, so the request only needs to
// signal "a viewer is watching" to keep the segment pipeline alive. Quality lets
// the viewer ask for the main (high) or sub (low) stream on demand; the agent
// switches the live session's source stream when it changes.
type RequestHLSStreamPayload struct {
	Timestamp int64  `json:"timestamp"`         // timestamp
	Quality   string `json:"quality,omitempty"` // "auto" | "high" | "low" (empty => auto)
}

// We received a request HD stream request
type RequestHDStreamPayload struct {
	Timestamp          int64  `json:"timestamp"`           // timestamp
	HubKey             string `json:"hub_key"`             // hub key
	SessionID          string `json:"session_id"`          // session id
	SessionDescription string `json:"session_description"` // session description
	Quality            string `json:"quality,omitempty"`   // "auto" | "high" | "low" (empty => auto)
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

type TriggerRelay struct {
	Timestamp int64  `json:"timestamp"` // timestamp
	DeviceId  string `json:"device_id"` // device id
	Token     string `json:"token"`     // token
}
