package webrtc

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kerberos-io/agent/machinery/src/capture"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/packets"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	pionWebRTC "github.com/pion/webrtc/v3"
	pionMedia "github.com/pion/webrtc/v3/pkg/media"
)

var (
	CandidatesMutex     sync.Mutex
	CandidateArrays     map[string](chan string)
	peerConnectionCount int64
	peerConnections     map[string]*pionWebRTC.PeerConnection
	//encoder             *ffmpeg.VideoEncoder
)

type WebRTC struct {
	Name                  string
	StunServers           []string
	TurnServers           []string
	TurnServersUsername   string
	TurnServersCredential string
	Timer                 *time.Timer
	PacketsCount          chan int
}

// No longer used, is for transcoding, might comeback on this!
/*func init() {
	// Encoder is created for once and for all.
	var err error
	encoder, err = ffmpeg.NewVideoEncoderByCodecType(av.H264)
	if err != nil {
		return
	}
	if encoder == nil {
		err = fmt.Errorf("Video encoder not found")
		return
	}
	encoder.SetFramerate(30, 1)
	encoder.SetPixelFormat(av.I420)
	encoder.SetBitrate(1000000) // 1MB
	encoder.SetGopSize(30 / 1)  // 1s
}*/

func CreateWebRTC(name string, stunServers []string, turnServers []string, turnServersUsername string, turnServersCredential string) *WebRTC {
	return &WebRTC{
		Name:                  name,
		StunServers:           stunServers,
		TurnServers:           turnServers,
		TurnServersUsername:   turnServersUsername,
		TurnServersCredential: turnServersCredential,
		Timer:                 time.NewTimer(time.Second * 10),
		PacketsCount:          make(chan int),
	}
}

func (w WebRTC) DecodeSessionDescription(data string) ([]byte, error) {
	sd, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		log.Log.Error("DecodeString error: " + err.Error())
		return []byte{}, err
	}
	return sd, nil
}

func (w WebRTC) CreateOffer(sd []byte) pionWebRTC.SessionDescription {
	offer := pionWebRTC.SessionDescription{
		Type: pionWebRTC.SDPTypeOffer,
		SDP:  string(sd),
	}
	return offer
}

func RegisterCandidates(key string, candidate models.ReceiveHDCandidatesPayload) {
	// Set lock
	CandidatesMutex.Lock()
	_, ok := CandidateArrays[key]
	if !ok {
		CandidateArrays[key] = make(chan string)
	}
	log.Log.Info("HandleReceiveHDCandidates: " + candidate.Candidate)
	select {
	case CandidateArrays[key] <- candidate.Candidate:
	default:
		log.Log.Info("HandleReceiveHDCandidates: channel is full.")
	}
	CandidatesMutex.Unlock()
}

func InitializeWebRTCConnection(configuration *models.Configuration, communication *models.Communication, mqttClient mqtt.Client, videoTrack *pionWebRTC.TrackLocalStaticSample, audioTrack *pionWebRTC.TrackLocalStaticSample, handshake models.RequestHDStreamPayload) {

	config := configuration.Config
	deviceKey := config.Key
	stunServers := []string{config.STUNURI}
	turnServers := []string{config.TURNURI}
	turnServersUsername := config.TURNUsername
	turnServersCredential := config.TURNPassword

	// We create a channel which will hold the candidates for this session.
	sessionKey := config.Key + "/" + handshake.SessionID
	CandidatesMutex.Lock()
	_, ok := CandidateArrays[sessionKey]
	if !ok {
		CandidateArrays[sessionKey] = make(chan string)
	}
	CandidatesMutex.Unlock()

	// Set variables
	hubKey := handshake.HubKey
	sessionDescription := handshake.SessionDescription

	// Create WebRTC object
	w := CreateWebRTC(deviceKey, stunServers, turnServers, turnServersUsername, turnServersCredential)
	sd, err := w.DecodeSessionDescription(sessionDescription)

	if err == nil {

		mediaEngine := &pionWebRTC.MediaEngine{}
		if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
			log.Log.Error("InitializeWebRTCConnection: something went wrong registering codecs.")
		}

		api := pionWebRTC.NewAPI(pionWebRTC.WithMediaEngine(mediaEngine))

		peerConnection, err := api.NewPeerConnection(
			pionWebRTC.Configuration{
				ICEServers: []pionWebRTC.ICEServer{
					{
						URLs: w.StunServers,
					},
					{
						URLs:       w.TurnServers,
						Username:   w.TurnServersUsername,
						Credential: w.TurnServersCredential,
					},
				},
				//ICETransportPolicy: pionWebRTC.ICETransportPolicyRelay,
			},
		)

		if err == nil && peerConnection != nil {

			if _, err = peerConnection.AddTrack(videoTrack); err != nil {
				//panic(err)
			}

			if _, err = peerConnection.AddTrack(audioTrack); err != nil {
				//panic(err)
			}

			if err != nil {
				//panic(err)
			}

			peerConnection.OnICEConnectionStateChange(func(connectionState pionWebRTC.ICEConnectionState) {
				if connectionState == pionWebRTC.ICEConnectionStateDisconnected {
					atomic.AddInt64(&peerConnectionCount, -1)

					// Set lock
					CandidatesMutex.Lock()
					peerConnections[handshake.SessionID] = nil
					_, ok := CandidateArrays[sessionKey]
					if ok {
						close(CandidateArrays[sessionKey])
					}
					CandidatesMutex.Unlock()

					close(w.PacketsCount)
					if err := peerConnection.Close(); err != nil {
						//panic(err)
					}
				} else if connectionState == pionWebRTC.ICEConnectionStateConnected {
					atomic.AddInt64(&peerConnectionCount, 1)
				} else if connectionState == pionWebRTC.ICEConnectionStateChecking {
					// Iterate over the candidates and send them to the remote client
					// Non blocking channel
					for candidate := range CandidateArrays[sessionKey] {
						log.Log.Info("InitializeWebRTCConnection: Received candidate.")
						if candidateErr := peerConnection.AddICECandidate(pionWebRTC.ICECandidateInit{Candidate: string(candidate)}); candidateErr != nil {
							log.Log.Error("InitializeWebRTCConnection: something went wrong while adding candidate: " + candidateErr.Error())
						}
					}
				} else if connectionState == pionWebRTC.ICEConnectionStateFailed {

				}
				log.Log.Info("InitializeWebRTCConnection: connection state changed to: " + connectionState.String())
				log.Log.Info("InitializeWebRTCConnection: Number of peers connected (" + strconv.FormatInt(peerConnectionCount, 10) + ")")
			})

			offer := w.CreateOffer(sd)
			if err = peerConnection.SetRemoteDescription(offer); err != nil {
				//panic(err)
			}

			answer, err := peerConnection.CreateAnswer(nil)
			if err != nil {
				//panic(err)
			} else if err = peerConnection.SetLocalDescription(answer); err != nil {
				//panic(err)
			}

			// When an ICE candidate is available send to the other peer using the signaling server (MQTT).
			// The other peer will add this candidate by calling AddICECandidate
			peerConnection.OnICECandidate(func(candidate *pionWebRTC.ICECandidate) {
				if candidate == nil {
					return
				}
				//  Create a config map
				valueMap := make(map[string]interface{})
				candateJSON := candidate.ToJSON()
				sdpmid := "0"
				candateJSON.SDPMid = &sdpmid
				candateBinary, err := json.Marshal(candateJSON)
				if err == nil {
					valueMap["candidate"] = string(candateBinary)
				} else {
					log.Log.Info("HandleRequestConfig: something went wrong while marshalling candidate: " + err.Error())
				}

				// We'll send the candidate to the hub
				message := models.Message{
					Payload: models.Payload{
						Action:   "receive-hd-candidates",
						DeviceId: configuration.Config.Key,
						Value:    valueMap,
					},
				}
				payload, err := models.PackageMQTTMessage(configuration, message)
				if err == nil {
					log.Log.Info("InitializeWebRTCConnection:" + string(candateBinary))
					token := mqttClient.Publish("kerberos/hub/"+hubKey, 2, false, payload)
					token.Wait()
				} else {
					log.Log.Info("HandleRequestConfig: something went wrong while sending acknowledge config to hub: " + string(payload))
				}
			})

			// Create a channel which will be used to send candidates to the other peer
			peerConnections[handshake.SessionID] = peerConnection

			if err == nil {
				//  Create a config map
				valueMap := make(map[string]interface{})
				valueMap["sdp"] = []byte(base64.StdEncoding.EncodeToString([]byte(answer.SDP)))
				log.Log.Info("InitializeWebRTCConnection: Send SDP answer")

				// We'll send the candidate to the hub
				message := models.Message{
					Payload: models.Payload{
						Action:   "receive-hd-answer",
						DeviceId: configuration.Config.Key,
						Value:    valueMap,
					},
				}
				payload, err := models.PackageMQTTMessage(configuration, message)
				if err == nil {
					token := mqttClient.Publish("kerberos/hub/"+hubKey, 2, false, payload)
					token.Wait()
				} else {
					log.Log.Info("HandleRequestConfig: something went wrong while sending acknowledge config to hub: " + string(payload))
				}
			}
		}
	} else {
		log.Log.Error("InitializeWebRTCConnection: NewPeerConnection failed: " + err.Error())
	}
}

func NewVideoTrack(streams []packets.Stream) *pionWebRTC.TrackLocalStaticSample {
	var mimeType string
	mimeType = pionWebRTC.MimeTypeH264
	outboundVideoTrack, _ := pionWebRTC.NewTrackLocalStaticSample(pionWebRTC.RTPCodecCapability{MimeType: mimeType}, "video", "pion124")
	return outboundVideoTrack
}

func NewAudioTrack(streams []packets.Stream) *pionWebRTC.TrackLocalStaticSample {
	var mimeType string
	for _, stream := range streams {
		if stream.Name == "OPUS" {
			mimeType = pionWebRTC.MimeTypeOpus
		} else if stream.Name == "PCM_MULAW" {
			mimeType = pionWebRTC.MimeTypePCMU
		} else if stream.Name == "PCM_ALAW" {
			mimeType = pionWebRTC.MimeTypePCMA
		}
	}
	outboundAudioTrack, _ := pionWebRTC.NewTrackLocalStaticSample(pionWebRTC.RTPCodecCapability{MimeType: mimeType}, "audio", "pion124")
	return outboundAudioTrack
}

func WriteToTrack(livestreamCursor *packets.QueueCursor, configuration *models.Configuration, communication *models.Communication, mqttClient mqtt.Client, videoTrack *pionWebRTC.TrackLocalStaticSample, audioTrack *pionWebRTC.TrackLocalStaticSample, rtspClient capture.RTSPClient) {

	config := configuration.Config

	// Make peerconnection map
	peerConnections = make(map[string]*pionWebRTC.PeerConnection)

	// Set the indexes for the video & audio streams
	// Later when we read a packet we need to figure out which track to send it to.
	videoIdx := -1
	audioIdx := -1
	streams, _ := rtspClient.GetStreams()
	for i, stream := range streams {
		if stream.Name == "H264" && videoIdx < 0 {
			videoIdx = i
		} else if (stream.Name == "OPUS" || stream.Name == "PCM_MULAW" || stream.Name == "PCM_ALAW") && audioIdx < 0 {
			audioIdx = i
		}
	}

	if videoIdx == -1 {
		log.Log.Error("WriteToTrack: no video codec found.")
	} else {
		if config.Capture.TranscodingWebRTC == "true" {
			if videoIdx > -1 {
				log.Log.Info("WriteToTrack: successfully using a transcoder.")
			} else {
			}
		} else {
			log.Log.Info("WriteToTrack: not using a transcoder.")
		}

		var cursorError error
		var pkt packets.Packet
		var previousTime time.Duration

		start := false
		receivedKeyFrame := false
		lastKeepAlive := "0"
		peerCount := "0"

		for cursorError == nil {

			pkt, cursorError = livestreamCursor.ReadPacket()
			bufferDuration := pkt.Time - previousTime
			previousTime = pkt.Time

			if config.Capture.ForwardWebRTC != "true" && peerConnectionCount == 0 {
				start = false
				receivedKeyFrame = false
				continue
			}

			select {
			case lastKeepAlive = <-communication.HandleLiveHDKeepalive:
			default:
			}

			select {
			case peerCount = <-communication.HandleLiveHDPeers:
			default:
			}

			now := time.Now().Unix()
			lastKeepAliveN, _ := strconv.ParseInt(lastKeepAlive, 10, 64)
			hasTimedOut := (now - lastKeepAliveN) > 15 // if longer then no response in 15 sec.
			hasNoPeers := peerCount == "0"

			if config.Capture.ForwardWebRTC == "true" && (hasTimedOut || hasNoPeers) {
				start = false
				receivedKeyFrame = false
				continue
			}

			if len(pkt.Data) == 0 || pkt.Data == nil {
				receivedKeyFrame = false
				continue
			}

			if !receivedKeyFrame {
				if pkt.IsKeyFrame {
					receivedKeyFrame = true
				} else {
					continue
				}
			}

			//if config.Capture.TranscodingWebRTC == "true" {
			// We will transcode the video
			// TODO..
			//}

			switch int(pkt.Idx) {
			case videoIdx:
				// Start at the first keyframe
				if pkt.IsKeyFrame {
					start = true
				}
				if start {
					sample := pionMedia.Sample{Data: pkt.Data, Duration: bufferDuration}
					if config.Capture.ForwardWebRTC == "true" {
						// We will send the video to a remote peer
						// TODO..
					} else {
						if err := videoTrack.WriteSample(sample); err != nil && err != io.ErrClosedPipe {
							log.Log.Error("WriteToTrack: something went wrong while writing sample: " + err.Error())
						}
					}
				}
			case audioIdx:
				// We will send the audio
				sample := pionMedia.Sample{Data: pkt.Data, Duration: pkt.Time}
				if err := audioTrack.WriteSample(sample); err != nil && err != io.ErrClosedPipe {
					log.Log.Error("WriteToTrack: something went wrong while writing sample: " + err.Error())
				}
			}
		}
	}
	for _, p := range peerConnections {
		if p != nil {
			p.Close()
		}
	}

	peerConnectionCount = 0
	log.Log.Info("WriteToTrack: stop writing to track.")
}
