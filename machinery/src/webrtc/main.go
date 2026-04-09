package webrtc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kerberos-io/agent/machinery/src/capture"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/packets"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/intervalpli"
	pionWebRTC "github.com/pion/webrtc/v4"
	pionMedia "github.com/pion/webrtc/v4/pkg/media"
)

const (
	// Channel buffer sizes
	candidateChannelBuffer = 100
	rtcpBufferSize         = 1500

	// Timeouts and intervals
	keepAliveTimeout      = 15 * time.Second
	defaultTimeout        = 10 * time.Second
	maxLivePacketAge      = 1500 * time.Millisecond
	disconnectGracePeriod = 5 * time.Second

	// Track identifiers
	trackStreamID = "kerberos-stream"
)

// ConnectionManager manages WebRTC peer connections in a thread-safe manner
type ConnectionManager struct {
	mu                  sync.RWMutex
	candidateChannels   map[string]chan string
	peerConnections     map[string]*peerConnectionWrapper
	peerConnectionCount int64
}

// peerConnectionWrapper wraps a peer connection with additional metadata
type peerConnectionWrapper struct {
	conn             *pionWebRTC.PeerConnection
	cancelCtx        context.CancelFunc
	done             chan struct{}
	closeOnce        sync.Once
	connected        atomic.Bool
	disconnectMu     sync.Mutex
	disconnectTimer  *time.Timer
	sessionKey       string
	videoBroadcaster *TrackBroadcaster
	audioBroadcaster *TrackBroadcaster
}

var globalConnectionManager = NewConnectionManager()

// NewConnectionManager creates a new connection manager
func NewConnectionManager() *ConnectionManager {
	return &ConnectionManager{
		candidateChannels: make(map[string]chan string),
		peerConnections:   make(map[string]*peerConnectionWrapper),
	}
}

// GetOrCreateCandidateChannel gets or creates a candidate channel for a session
func (cm *ConnectionManager) GetOrCreateCandidateChannel(sessionKey string) chan string {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if ch, exists := cm.candidateChannels[sessionKey]; exists {
		return ch
	}

	ch := make(chan string, candidateChannelBuffer)
	cm.candidateChannels[sessionKey] = ch
	return ch
}

// CloseCandidateChannel safely closes and removes a candidate channel
func (cm *ConnectionManager) CloseCandidateChannel(sessionKey string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if ch, exists := cm.candidateChannels[sessionKey]; exists {
		close(ch)
		delete(cm.candidateChannels, sessionKey)
	}
}

// AddPeerConnection adds a peer connection to the manager
func (cm *ConnectionManager) AddPeerConnection(sessionKey string, wrapper *peerConnectionWrapper) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.peerConnections[sessionKey] = wrapper
}

// RemovePeerConnection removes a peer connection from the manager
func (cm *ConnectionManager) RemovePeerConnection(sessionKey string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if wrapper, exists := cm.peerConnections[sessionKey]; exists {
		if wrapper.cancelCtx != nil {
			wrapper.cancelCtx()
		}
		delete(cm.peerConnections, sessionKey)
	}
}

// QueueCandidate safely queues a candidate for a session without racing with channel closure.
func (cm *ConnectionManager) QueueCandidate(sessionKey string, candidate string) bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	ch, exists := cm.candidateChannels[sessionKey]
	if !exists {
		ch = make(chan string, candidateChannelBuffer)
		cm.candidateChannels[sessionKey] = ch
	}

	select {
	case ch <- candidate:
		return true
	default:
		return false
	}
}

// GetPeerConnectionCount returns the current count of active peer connections
func (cm *ConnectionManager) GetPeerConnectionCount() int64 {
	return atomic.LoadInt64(&cm.peerConnectionCount)
}

// GetActivePeerConnectionCount returns the current number of connected WebRTC readers.
func GetActivePeerConnectionCount() int64 {
	return globalConnectionManager.GetPeerConnectionCount()
}

// IncrementPeerCount atomically increments the peer connection count
func (cm *ConnectionManager) IncrementPeerCount() int64 {
	return atomic.AddInt64(&cm.peerConnectionCount, 1)
}

// DecrementPeerCount atomically decrements the peer connection count
func (cm *ConnectionManager) DecrementPeerCount() int64 {
	return atomic.AddInt64(&cm.peerConnectionCount, -1)
}

func cleanupPeerConnection(sessionKey string, wrapper *peerConnectionWrapper) {
	wrapper.closeOnce.Do(func() {
		if wrapper.connected.Swap(false) {
			count := globalConnectionManager.DecrementPeerCount()
			log.Log.Info("webrtc.main.cleanupPeerConnection(): Peer disconnected. Active peers: " + strconv.FormatInt(count, 10))
		}

		// Remove per-peer tracks from broadcasters so the fan-out stops
		// writing to this peer immediately.
		if wrapper.videoBroadcaster != nil {
			wrapper.videoBroadcaster.RemovePeer(sessionKey)
		}
		if wrapper.audioBroadcaster != nil {
			wrapper.audioBroadcaster.RemovePeer(sessionKey)
		}

		globalConnectionManager.CloseCandidateChannel(sessionKey)

		if wrapper.conn != nil {
			if err := wrapper.conn.Close(); err != nil {
				log.Log.Error("webrtc.main.cleanupPeerConnection(): error closing peer connection: " + err.Error())
			}
		}

		globalConnectionManager.RemovePeerConnection(sessionKey)
		close(wrapper.done)
	})
}

type WebRTC struct {
	Name                  string
	StunServers           []string
	TurnServers           []string
	TurnServersUsername   string
	TurnServersCredential string
	Timer                 *time.Timer
	PacketsCount          chan int
}

func CreateWebRTC(name string, stunServers []string, turnServers []string, turnServersUsername string, turnServersCredential string) *WebRTC {
	return &WebRTC{
		Name:                  name,
		StunServers:           stunServers,
		TurnServers:           turnServers,
		TurnServersUsername:   turnServersUsername,
		TurnServersCredential: turnServersCredential,
		Timer:                 time.NewTimer(defaultTimeout),
	}
}

func (w WebRTC) DecodeSessionDescription(data string) ([]byte, error) {
	sd, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		log.Log.Error("webrtc.main.DecodeSessionDescription(): " + err.Error())
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
	log.Log.Info("webrtc.main.RegisterCandidates(): " + candidate.Candidate)
	if !globalConnectionManager.QueueCandidate(key, candidate.Candidate) {
		log.Log.Info("webrtc.main.RegisterCandidates(): channel is full, dropping candidate")
	}
}

func decodeICECandidate(candidate string) (pionWebRTC.ICECandidateInit, error) {
	if candidate == "" {
		return pionWebRTC.ICECandidateInit{}, io.EOF
	}

	var candidateInit pionWebRTC.ICECandidateInit
	if err := json.Unmarshal([]byte(candidate), &candidateInit); err == nil {
		if candidateInit.Candidate != "" {
			return candidateInit, nil
		}
	}

	return pionWebRTC.ICECandidateInit{Candidate: candidate}, nil
}

func RegisterDefaultInterceptors(mediaEngine *pionWebRTC.MediaEngine, interceptorRegistry *interceptor.Registry) error {
	if err := pionWebRTC.ConfigureNack(mediaEngine, interceptorRegistry); err != nil {
		return err
	}
	if err := pionWebRTC.ConfigureRTCPReports(interceptorRegistry); err != nil {
		return err
	}
	if err := pionWebRTC.ConfigureSimulcastExtensionHeaders(mediaEngine); err != nil {
		return err
	}
	return nil
}

func publishSignalingMessageAsync(mqttClient mqtt.Client, topic string, payload []byte, description string) {
	if mqttClient == nil {
		log.Log.Error("webrtc.main.publishSignalingMessageAsync(): mqtt client is nil for " + description)
		return
	}

	token := mqttClient.Publish(topic, 2, false, payload)
	go func() {
		if !token.WaitTimeout(5 * time.Second) {
			log.Log.Warning("webrtc.main.publishSignalingMessageAsync(): timed out publishing " + description)
			return
		}
		if err := token.Error(); err != nil {
			log.Log.Error("webrtc.main.publishSignalingMessageAsync(): failed publishing " + description + ": " + err.Error())
		}
	}()
}

func sendCandidateSignal(configuration *models.Configuration, mqttClient mqtt.Client, hubKey string, handshake models.LiveHDHandshake, candidateJSON []byte) {
	if handshake.Signaling != nil && handshake.Signaling.SendCandidate != nil {
		if err := handshake.Signaling.SendCandidate(handshake.Payload.SessionID, string(candidateJSON)); err != nil {
			log.Log.Error("webrtc.main.sendCandidateSignal(): " + err.Error())
		}
		return
	}

	message := models.Message{
		Payload: models.Payload{
			Action:   "receive-hd-candidates",
			DeviceId: configuration.Config.Key,
			Value: map[string]interface{}{
				"candidate":  string(candidateJSON),
				"session_id": handshake.Payload.SessionID,
			},
		},
	}
	payload, err := models.PackageMQTTMessage(configuration, message)
	if err == nil {
		publishSignalingMessageAsync(mqttClient, "kerberos/hub/"+hubKey, payload, "ICE candidate for session "+handshake.Payload.SessionID)
	} else {
		log.Log.Info("webrtc.main.sendCandidateSignal(): while packaging mqtt message: " + err.Error())
	}
}

func sendAnswerSignal(configuration *models.Configuration, mqttClient mqtt.Client, hubKey string, handshake models.LiveHDHandshake, answer pionWebRTC.SessionDescription) {
	encodedAnswer := base64.StdEncoding.EncodeToString([]byte(answer.SDP))

	if handshake.Signaling != nil && handshake.Signaling.SendAnswer != nil {
		if err := handshake.Signaling.SendAnswer(handshake.Payload.SessionID, encodedAnswer); err != nil {
			log.Log.Error("webrtc.main.sendAnswerSignal(): " + err.Error())
		}
		return
	}

	message := models.Message{
		Payload: models.Payload{
			Action:   "receive-hd-answer",
			DeviceId: configuration.Config.Key,
			Value: map[string]interface{}{
				"sdp":        []byte(encodedAnswer),
				"session_id": handshake.Payload.SessionID,
			},
		},
	}
	payload, err := models.PackageMQTTMessage(configuration, message)
	if err == nil {
		publishSignalingMessageAsync(mqttClient, "kerberos/hub/"+hubKey, payload, "SDP answer for session "+handshake.Payload.SessionID)
	} else {
		log.Log.Info("webrtc.main.sendAnswerSignal(): while packaging mqtt message: " + err.Error())
	}
}

func InitializeWebRTCConnection(configuration *models.Configuration, communication *models.Communication, mqttClient mqtt.Client, videoBroadcaster *TrackBroadcaster, audioBroadcaster *TrackBroadcaster, handshake models.LiveHDHandshake) {

	config := configuration.Config
	deviceKey := config.Key
	stunServers := []string{config.STUNURI}
	turnServers := []string{config.TURNURI}
	turnServersUsername := config.TURNUsername
	turnServersCredential := config.TURNPassword
	handshakePayload := handshake.Payload

	// We create a channel which will hold the candidates for this session.
	sessionKey := config.Key + "/" + handshakePayload.SessionID
	candidateChannel := globalConnectionManager.GetOrCreateCandidateChannel(sessionKey)

	// Set variables
	hubKey := handshakePayload.HubKey
	sessionDescription := handshakePayload.SessionDescription

	// Create WebRTC object
	w := CreateWebRTC(deviceKey, stunServers, turnServers, turnServersUsername, turnServersCredential)
	sd, err := w.DecodeSessionDescription(sessionDescription)

	if err == nil {

		mediaEngine := &pionWebRTC.MediaEngine{}
		if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
			log.Log.Error("webrtc.main.InitializeWebRTCConnection(): something went wrong registering codecs for media engine: " + err.Error())
		}

		// Create a InterceptorRegistry. This is the user configurable RTP/RTCP Pipeline.
		// This provides NACKs, RTCP Reports and other features. If you use `webrtc.NewPeerConnection`
		// this is enabled by default. If you are manually managing You MUST create a InterceptorRegistry
		// for each PeerConnection.
		interceptorRegistry := &interceptor.Registry{}

		// Use the default set of Interceptors
		if err := pionWebRTC.RegisterDefaultInterceptors(mediaEngine, interceptorRegistry); err != nil {
			panic(err)
		}

		// Register a intervalpli factory
		// This interceptor sends a PLI every 3 seconds. A PLI causes a video keyframe to be generated by the sender.
		// This makes our video seekable and more error resilent, but at a cost of lower picture quality and higher bitrates
		// A real world application should process incoming RTCP packets from viewers and forward them to senders
		intervalPliFactory, err := intervalpli.NewReceiverInterceptor()
		if err != nil {
			panic(err)
		}
		interceptorRegistry.Add(intervalPliFactory)

		api := pionWebRTC.NewAPI(
			pionWebRTC.WithMediaEngine(mediaEngine),
			pionWebRTC.WithInterceptorRegistry(interceptorRegistry),
		)

		policy := pionWebRTC.ICETransportPolicyAll
		if config.ForceTurn == "true" {
			policy = pionWebRTC.ICETransportPolicyRelay
		}

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
				ICETransportPolicy: policy,
			},
		)

		if err == nil && peerConnection != nil {

			// Create context for this connection
			ctx, cancel := context.WithCancel(context.Background())
			wrapper := &peerConnectionWrapper{
				conn:             peerConnection,
				cancelCtx:        cancel,
				done:             make(chan struct{}),
				sessionKey:       sessionKey,
				videoBroadcaster: videoBroadcaster,
				audioBroadcaster: audioBroadcaster,
			}

			// Create a per-peer video track from the broadcaster so writes
			// to this peer are independent and non-blocking.
			var videoSender *pionWebRTC.RTPSender = nil
			if videoBroadcaster != nil {
				peerVideoTrack, trackErr := videoBroadcaster.AddPeer(sessionKey)
				if trackErr != nil {
					log.Log.Error("webrtc.main.InitializeWebRTCConnection(): error creating per-peer video track: " + trackErr.Error())
					cleanupPeerConnection(sessionKey, wrapper)
					return
				}
				if videoSender, err = peerConnection.AddTrack(peerVideoTrack); err != nil {
					log.Log.Error("webrtc.main.InitializeWebRTCConnection(): error adding video track: " + err.Error())
					cleanupPeerConnection(sessionKey, wrapper)
					return
				}
			} else {
				log.Log.Info("webrtc.main.InitializeWebRTCConnection(): video track is nil, skipping video")
			}

			// Read incoming RTCP packets
			// Before these packets are returned they are processed by interceptors. For things
			// like NACK this needs to be called.
			if videoSender != nil {
				go func() {
					defer func() {
						log.Log.Info("webrtc.main.InitializeWebRTCConnection(): video RTCP reader stopped")
					}()
					rtcpBuf := make([]byte, rtcpBufferSize)
					for {
						select {
						case <-ctx.Done():
							return
						default:
							if _, _, rtcpErr := videoSender.Read(rtcpBuf); rtcpErr != nil {
								return
							}
						}
					}
				}()
			}

			// Create a per-peer audio track from the broadcaster.
			var audioSender *pionWebRTC.RTPSender = nil
			if audioBroadcaster != nil {
				peerAudioTrack, trackErr := audioBroadcaster.AddPeer(sessionKey)
				if trackErr != nil {
					log.Log.Error("webrtc.main.InitializeWebRTCConnection(): error creating per-peer audio track: " + trackErr.Error())
					cleanupPeerConnection(sessionKey, wrapper)
					return
				}
				if audioSender, err = peerConnection.AddTrack(peerAudioTrack); err != nil {
					log.Log.Error("webrtc.main.InitializeWebRTCConnection(): error adding audio track: " + err.Error())
					cleanupPeerConnection(sessionKey, wrapper)
					return
				}
			} else {
				log.Log.Info("webrtc.main.InitializeWebRTCConnection(): audio track is nil, skipping audio")
			}

			// Read incoming RTCP packets
			// Before these packets are returned they are processed by interceptors. For things
			// like NACK this needs to be called.
			if audioSender != nil {
				go func() {
					defer func() {
						log.Log.Info("webrtc.main.InitializeWebRTCConnection(): audio RTCP reader stopped")
					}()
					rtcpBuf := make([]byte, rtcpBufferSize)
					for {
						select {
						case <-ctx.Done():
							return
						default:
							if _, _, rtcpErr := audioSender.Read(rtcpBuf); rtcpErr != nil {
								return
							}
						}
					}
				}()
			}

			// Log ICE connection state changes for diagnostics
			peerConnection.OnICEConnectionStateChange(func(iceState pionWebRTC.ICEConnectionState) {
				log.Log.Info("webrtc.main.InitializeWebRTCConnection(): ICE connection state changed to: " + iceState.String() +
					" (session: " + handshakePayload.SessionID + ")")
			})

			peerConnection.OnConnectionStateChange(func(connectionState pionWebRTC.PeerConnectionState) {
				log.Log.Info("webrtc.main.InitializeWebRTCConnection(): connection state changed to: " + connectionState.String() +
					" (session: " + handshakePayload.SessionID + ")")

				switch connectionState {
				case pionWebRTC.PeerConnectionStateDisconnected:
					// Disconnected is a transient state that can recover.
					// Start a grace period timer; if we don't recover, then cleanup.
					wrapper.disconnectMu.Lock()
					if wrapper.disconnectTimer == nil {
						log.Log.Info("webrtc.main.InitializeWebRTCConnection(): peer disconnected, waiting " +
							disconnectGracePeriod.String() + " for recovery (session: " + handshakePayload.SessionID + ")")
						wrapper.disconnectTimer = time.AfterFunc(disconnectGracePeriod, func() {
							log.Log.Info("webrtc.main.InitializeWebRTCConnection(): disconnect grace period expired, closing connection (session: " + handshakePayload.SessionID + ")")
							cleanupPeerConnection(sessionKey, wrapper)
						})
					}
					wrapper.disconnectMu.Unlock()

				case pionWebRTC.PeerConnectionStateFailed:
					// Stop any pending disconnect timer
					wrapper.disconnectMu.Lock()
					if wrapper.disconnectTimer != nil {
						wrapper.disconnectTimer.Stop()
						wrapper.disconnectTimer = nil
					}
					wrapper.disconnectMu.Unlock()
					cleanupPeerConnection(sessionKey, wrapper)

				case pionWebRTC.PeerConnectionStateClosed:
					// Stop any pending disconnect timer
					wrapper.disconnectMu.Lock()
					if wrapper.disconnectTimer != nil {
						wrapper.disconnectTimer.Stop()
						wrapper.disconnectTimer = nil
					}
					wrapper.disconnectMu.Unlock()
					cleanupPeerConnection(sessionKey, wrapper)

				case pionWebRTC.PeerConnectionStateConnected:
					// Cancel any pending disconnect timer — connection recovered
					wrapper.disconnectMu.Lock()
					if wrapper.disconnectTimer != nil {
						wrapper.disconnectTimer.Stop()
						wrapper.disconnectTimer = nil
						log.Log.Info("webrtc.main.InitializeWebRTCConnection(): connection recovered from disconnected state (session: " + handshakePayload.SessionID + ")")
					}
					wrapper.disconnectMu.Unlock()

					if wrapper.connected.CompareAndSwap(false, true) {
						count := globalConnectionManager.IncrementPeerCount()
						log.Log.Info("webrtc.main.InitializeWebRTCConnection(): Peer connected. Active peers: " + strconv.FormatInt(count, 10))
					}
				}
			})

			// When an ICE candidate is available send to the other peer using the signaling server (MQTT).
			// The other peer will add this candidate by calling AddICECandidate.
			// This handler must be registered before setting the local description, otherwise early candidates can be missed.
			var hasRelayCandidates bool
			peerConnection.OnICECandidate(func(candidate *pionWebRTC.ICECandidate) {

				if candidate == nil {
					log.Log.Info("webrtc.main.InitializeWebRTCConnection(): ICE gathering complete (candidate is nil)")
					if !hasRelayCandidates {
						log.Log.Error("webrtc.main.InitializeWebRTCConnection(): WARNING - No TURN (relay) candidates were gathered! TURN servers: " +
							config.TURNURI + ", Username: " + config.TURNUsername + ", ForceTurn: " + config.ForceTurn)
					}
					return
				}

				// Log candidate details for debugging
				candidateJSON := candidate.ToJSON()
				candidateStr := candidateJSON.Candidate

				// Determine candidate type from the candidate string
				candidateType := "unknown"
				if candidateJSON.Candidate != "" {
					switch candidate.Typ {
					case pionWebRTC.ICECandidateTypeRelay:
						candidateType = "relay"
					case pionWebRTC.ICECandidateTypeSrflx:
						candidateType = "srflx"
					case pionWebRTC.ICECandidateTypeHost:
						candidateType = "host"
					case pionWebRTC.ICECandidateTypePrflx:
						candidateType = "prflx"
					}
				}

				// Track if we received any relay (TURN) candidates
				if candidateType == "relay" {
					hasRelayCandidates = true
				}

				log.Log.Info("webrtc.main.InitializeWebRTCConnection(): ICE candidate received - Type: " + candidateType +
					", Candidate: " + candidateStr)

				//  Create a config map
				valueMap := make(map[string]interface{})
				candateBinary, err := json.Marshal(candidateJSON)
				if err == nil {
					valueMap["candidate"] = string(candateBinary)
					valueMap["session_id"] = handshakePayload.SessionID
					log.Log.Info("webrtc.main.InitializeWebRTCConnection(): sending " + candidateType + " candidate to hub")
				} else {
					log.Log.Error("webrtc.main.InitializeWebRTCConnection(): failed to marshal candidate: " + err.Error())
				}

				sendCandidateSignal(configuration, mqttClient, hubKey, handshake, candateBinary)
			})

			offer := w.CreateOffer(sd)
			if err = peerConnection.SetRemoteDescription(offer); err != nil {
				log.Log.Error("webrtc.main.InitializeWebRTCConnection(): something went wrong while setting remote description: " + err.Error())
				cleanupPeerConnection(sessionKey, wrapper)
				return
			}

			go func() {
				defer func() {
					log.Log.Info("webrtc.main.InitializeWebRTCConnection(): candidate processor stopped for session: " + handshakePayload.SessionID)
				}()

				// Process remote candidates only after the remote description is set.
				// MQTT can deliver candidates before the SDP offer handling completes,
				// and Pion rejects AddICECandidate calls until SetRemoteDescription succeeds.
				for {
					select {
					case <-ctx.Done():
						return
					case candidate, ok := <-candidateChannel:
						if !ok {
							return
						}
						log.Log.Info("webrtc.main.InitializeWebRTCConnection(): Received candidate from channel: " + candidate)
						candidateInit, decodeErr := decodeICECandidate(candidate)
						if decodeErr != nil {
							log.Log.Error("webrtc.main.InitializeWebRTCConnection(): error decoding candidate: " + decodeErr.Error())
							continue
						}
						if candidateErr := peerConnection.AddICECandidate(candidateInit); candidateErr != nil {
							log.Log.Error("webrtc.main.InitializeWebRTCConnection(): error adding candidate: " + candidateErr.Error())
						}
					}
				}
			}()

			answer, err := peerConnection.CreateAnswer(nil)
			if err != nil {
				log.Log.Error("webrtc.main.InitializeWebRTCConnection(): something went wrong while creating answer: " + err.Error())
				cleanupPeerConnection(sessionKey, wrapper)
				return
			} else if err = peerConnection.SetLocalDescription(answer); err != nil {
				log.Log.Error("webrtc.main.InitializeWebRTCConnection(): something went wrong while setting local description: " + err.Error())
				cleanupPeerConnection(sessionKey, wrapper)
				return
			}

			// Store peer connection in manager
			globalConnectionManager.AddPeerConnection(sessionKey, wrapper)

			log.Log.Info("webrtc.main.InitializeWebRTCConnection(): Send SDP answer")

			sendAnswerSignal(configuration, mqttClient, hubKey, handshake, answer)
		}
	} else {
		globalConnectionManager.CloseCandidateChannel(sessionKey)
		log.Log.Error("webrtc.main.InitializeWebRTCConnection(): failed to decode remote session description: " + err.Error())
	}
}

func NewVideoBroadcaster(streams []packets.Stream) *TrackBroadcaster {
	// Verify H264 is available (same check as NewVideoTrack)
	for _, s := range streams {
		if s.Name == "H264" {
			return NewTrackBroadcaster(pionWebRTC.MimeTypeH264, "video", trackStreamID)
		}
	}
	log.Log.Error("webrtc.main.NewVideoBroadcaster(): no H264 stream found")
	return nil
}

func NewAudioBroadcaster(streams []packets.Stream) *TrackBroadcaster {
	var audioCodecNames []string
	hasAAC := false
	for _, s := range streams {
		if s.IsAudio {
			audioCodecNames = append(audioCodecNames, s.Name)
		}
		switch s.Name {
		case "OPUS":
			return NewTrackBroadcaster(pionWebRTC.MimeTypeOpus, "audio", trackStreamID)
		case "PCM_MULAW":
			return NewTrackBroadcaster(pionWebRTC.MimeTypePCMU, "audio", trackStreamID)
		case "PCM_ALAW":
			return NewTrackBroadcaster(pionWebRTC.MimeTypePCMA, "audio", trackStreamID)
		case "AAC":
			hasAAC = true
		}
	}
	if hasAAC {
		log.Log.Info("webrtc.main.NewAudioBroadcaster(): AAC detected, creating PCMU audio track for transcoded output")
		return NewTrackBroadcaster(pionWebRTC.MimeTypePCMU, "audio", trackStreamID)
	} else if len(audioCodecNames) > 0 {
		log.Log.Error(fmt.Sprintf("webrtc.main.NewAudioBroadcaster(): no supported audio codec found (detected: %s; supported: OPUS, PCM_MULAW, PCM_ALAW)", strings.Join(audioCodecNames, ", ")))
	} else {
		log.Log.Info("webrtc.main.NewAudioBroadcaster(): no audio stream found in camera feed")
	}
	return nil
}

func NewVideoTrack(streams []packets.Stream) *pionWebRTC.TrackLocalStaticSample {
	mimeType := pionWebRTC.MimeTypeH264
	outboundVideoTrack, err := pionWebRTC.NewTrackLocalStaticSample(pionWebRTC.RTPCodecCapability{MimeType: mimeType}, "video", trackStreamID)
	if err != nil {
		log.Log.Error("webrtc.main.NewVideoTrack(): error creating video track: " + err.Error())
		return nil
	}
	return outboundVideoTrack
}

func NewAudioTrack(streams []packets.Stream) *pionWebRTC.TrackLocalStaticSample {
	var mimeType string
	var audioCodecNames []string
	hasAAC := false
	for _, stream := range streams {
		if stream.IsAudio {
			audioCodecNames = append(audioCodecNames, stream.Name)
		}
		if stream.Name == "OPUS" {
			mimeType = pionWebRTC.MimeTypeOpus
		} else if stream.Name == "PCM_MULAW" {
			mimeType = pionWebRTC.MimeTypePCMU
		} else if stream.Name == "PCM_ALAW" {
			mimeType = pionWebRTC.MimeTypePCMA
		} else if stream.Name == "AAC" {
			hasAAC = true
		}
	}
	if mimeType == "" {
		if hasAAC {
			mimeType = pionWebRTC.MimeTypePCMU
			log.Log.Info("webrtc.main.NewAudioTrack(): AAC detected, creating PCMU audio track for transcoded output")
		} else if len(audioCodecNames) > 0 {
			log.Log.Error(fmt.Sprintf("webrtc.main.NewAudioTrack(): no supported audio codec found (detected: %s; supported: OPUS, PCM_MULAW, PCM_ALAW)", strings.Join(audioCodecNames, ", ")))
			return nil
		} else {
			log.Log.Info("webrtc.main.NewAudioTrack(): no audio stream found in camera feed")
			return nil
		}
	}
	outboundAudioTrack, err := pionWebRTC.NewTrackLocalStaticSample(pionWebRTC.RTPCodecCapability{MimeType: mimeType}, "audio", trackStreamID)
	if err != nil {
		log.Log.Error("webrtc.main.NewAudioTrack(): error creating audio track: " + err.Error())
		return nil
	}
	return outboundAudioTrack
}

// streamState holds state information for the streaming process
type streamState struct {
	lastKeepAlive    int64
	peerCount        int64
	start            bool
	catchingUp       bool
	receivedKeyFrame bool
	lastAudioSample  *pionMedia.Sample
	lastVideoSample  *pionMedia.Sample
	audioPacketsSeen int64
	aacPacketsSeen   int64
	audioSamplesSent int64
	aacNoOutput      int64
	aacErrors        int64
}

// codecSupport tracks which codecs are available in the stream
type codecSupport struct {
	hasH264      bool
	hasPCM_MULAW bool
	hasAAC       bool
	hasOpus      bool
}

// detectCodecs examines the stream to determine which codecs are available
func detectCodecs(rtspClient capture.RTSPClient) codecSupport {
	support := codecSupport{}
	streams, _ := rtspClient.GetStreams()

	for _, stream := range streams {
		switch stream.Name {
		case "H264":
			support.hasH264 = true
		case "PCM_MULAW":
			support.hasPCM_MULAW = true
		case "AAC":
			support.hasAAC = true
		case "OPUS":
			support.hasOpus = true
		}
	}

	return support
}

// hasValidCodecs checks if at least one valid video or audio codec is present
func (cs codecSupport) hasValidCodecs() bool {
	hasVideo := cs.hasH264
	hasAudio := cs.hasPCM_MULAW || cs.hasAAC || cs.hasOpus
	return hasVideo || hasAudio
}

// shouldContinueStreaming determines if streaming should continue based on keepalive and peer count
func shouldContinueStreaming(config models.Config, state *streamState) bool {
	if config.Capture.ForwardWebRTC != "true" {
		return true
	}

	now := time.Now().Unix()
	hasTimedOut := (now - state.lastKeepAlive) > int64(keepAliveTimeout.Seconds())
	hasNoPeers := state.peerCount == 0

	return !hasTimedOut && !hasNoPeers
}

// updateStreamState updates keepalive and peer count from communication channels
func updateStreamState(communication *models.Communication, state *streamState) {
	select {
	case keepAliveStr := <-communication.HandleLiveHDKeepalive:
		if val, err := strconv.ParseInt(keepAliveStr, 10, 64); err == nil {
			state.lastKeepAlive = val
		}
	default:
	}

	select {
	case peerCountStr := <-communication.HandleLiveHDPeers:
		if val, err := strconv.ParseInt(peerCountStr, 10, 64); err == nil {
			state.peerCount = val
		}
	default:
	}
}

// writeFinalSamples writes any remaining buffered samples
func writeFinalSamples(state *streamState, videoBroadcaster, audioBroadcaster *TrackBroadcaster) {
	if state.lastVideoSample != nil && videoBroadcaster != nil {
		videoBroadcaster.WriteSample(*state.lastVideoSample)
	}

	if state.lastAudioSample != nil && audioBroadcaster != nil {
		audioBroadcaster.WriteSample(*state.lastAudioSample)
	}
}

func sampleTimestamp(pkt packets.Packet) uint32 {
	if pkt.TimeLegacy > 0 {
		return uint32(pkt.TimeLegacy.Milliseconds())
	}

	if pkt.Time > 0 {
		return uint32(pkt.Time)
	}

	return 0
}

func sampleDuration(current packets.Packet, previousTimestamp uint32, fallback time.Duration) time.Duration {
	if current.TimeLegacy > 0 {
		currentDurationMs := current.TimeLegacy.Milliseconds()
		previousDurationMs := int64(previousTimestamp)
		if currentDurationMs > previousDurationMs {
			duration := time.Duration(currentDurationMs-previousDurationMs) * time.Millisecond
			if duration > 0 {
				return duration
			}
		}
	}

	currentTimestamp := sampleTimestamp(current)
	if currentTimestamp > previousTimestamp {
		duration := time.Duration(currentTimestamp-previousTimestamp) * time.Millisecond
		if duration > 0 {
			return duration
		}
	}

	return fallback
}

// processVideoPacket processes a video packet and writes samples to the broadcaster
func processVideoPacket(pkt packets.Packet, state *streamState, videoBroadcaster *TrackBroadcaster, config models.Config) {
	if videoBroadcaster == nil {
		return
	}

	// Start at the first keyframe
	if pkt.IsKeyFrame {
		state.start = true
	}

	if !state.start {
		return
	}

	sample := pionMedia.Sample{Data: pkt.Data, PacketTimestamp: sampleTimestamp(pkt)}

	if config.Capture.ForwardWebRTC == "true" {
		// Remote forwarding not yet implemented
		log.Log.Debug("webrtc.main.processVideoPacket(): remote forwarding not implemented")
		return
	}

	if state.lastVideoSample != nil {
		state.lastVideoSample.Duration = sampleDuration(pkt, state.lastVideoSample.PacketTimestamp, 33*time.Millisecond)
		videoBroadcaster.WriteSample(*state.lastVideoSample)
	}

	state.lastVideoSample = &sample
}

// processAudioPacket processes an audio packet and writes samples to the broadcaster.
// When the packet carries AAC and a transcoder is provided, the audio is transcoded
// to G.711 µ-law on the fly so it can be sent over a PCMU WebRTC track.
func processAudioPacket(pkt packets.Packet, state *streamState, audioBroadcaster *TrackBroadcaster, transcoder *AACTranscoder) {
	if audioBroadcaster == nil {
		return
	}

	state.audioPacketsSeen++

	audioData := pkt.Data

	if pkt.Codec == "AAC" {
		state.aacPacketsSeen++
		if transcoder == nil {
			state.aacErrors++
			if state.aacErrors <= 3 || state.aacErrors%100 == 0 {
				log.Log.Warning(fmt.Sprintf("webrtc.main.processAudioPacket(): AAC packet dropped because transcoder is nil (aac_packets=%d, input_bytes=%d)", state.aacPacketsSeen, len(pkt.Data)))
			}
			return // no transcoder – silently drop
		}
		pcmu, err := transcoder.Transcode(pkt.Data)
		if err != nil {
			state.aacErrors++
			log.Log.Error("webrtc.main.processAudioPacket(): AAC transcode error: " + err.Error())
			return
		}
		if len(pcmu) == 0 {
			state.aacNoOutput++
			if state.aacNoOutput <= 5 || state.aacNoOutput%100 == 0 {
				log.Log.Debug(fmt.Sprintf("webrtc.main.processAudioPacket(): AAC packet produced no PCMU output yet (aac_packets=%d, no_output=%d, input_bytes=%d)", state.aacPacketsSeen, state.aacNoOutput, len(pkt.Data)))
			}
			return // decoder still buffering
		}
		if state.aacPacketsSeen <= 5 || state.aacPacketsSeen%100 == 0 {
			log.Log.Info(fmt.Sprintf("webrtc.main.processAudioPacket(): AAC transcoded to PCMU (aac_packets=%d, input_bytes=%d, output_bytes=%d, peers=%d)", state.aacPacketsSeen, len(pkt.Data), len(pcmu), audioBroadcaster.PeerCount()))
		}
		audioData = pcmu
	}

	sample := pionMedia.Sample{Data: audioData, PacketTimestamp: sampleTimestamp(pkt)}

	if state.lastAudioSample != nil {
		state.lastAudioSample.Duration = sampleDuration(pkt, state.lastAudioSample.PacketTimestamp, 20*time.Millisecond)
		state.audioSamplesSent++
		if state.audioSamplesSent <= 5 || state.audioSamplesSent%100 == 0 {
			log.Log.Debug(fmt.Sprintf("webrtc.main.processAudioPacket(): queueing audio sample (samples=%d, codec=%s, bytes=%d, duration_ms=%d, peers=%d)", state.audioSamplesSent, pkt.Codec, len(state.lastAudioSample.Data), state.lastAudioSample.Duration.Milliseconds(), audioBroadcaster.PeerCount()))
		}
		audioBroadcaster.WriteSample(*state.lastAudioSample)
	}

	state.lastAudioSample = &sample
}

func shouldDropPacketForLatency(pkt packets.Packet) bool {
	if pkt.CurrentTime == 0 {
		return false
	}

	age := time.Since(time.UnixMilli(pkt.CurrentTime))
	return age > maxLivePacketAge
}

func WriteToTrack(livestreamCursor *packets.QueueCursor, configuration *models.Configuration, communication *models.Communication, mqttClient mqtt.Client, videoBroadcaster *TrackBroadcaster, audioBroadcaster *TrackBroadcaster, rtspClient capture.RTSPClient) {

	config := configuration.Config

	// Check if at least one broadcaster is available
	if videoBroadcaster == nil && audioBroadcaster == nil {
		log.Log.Error("webrtc.main.WriteToTrack(): both video and audio broadcasters are nil, cannot proceed")
		return
	}

	// Detect available codecs
	codecs := detectCodecs(rtspClient)

	if !codecs.hasValidCodecs() {
		log.Log.Error("webrtc.main.WriteToTrack(): no valid video or audio codec found")
		return
	}

	// Create AAC transcoder if needed (AAC → G.711 µ-law).
	var aacTranscoder *AACTranscoder
	if codecs.hasAAC && audioBroadcaster != nil {
		log.Log.Info(fmt.Sprintf("webrtc.main.WriteToTrack(): AAC audio detected, creating transcoder (audio_peers=%d)", audioBroadcaster.PeerCount()))
		t, err := NewAACTranscoder()
		if err != nil {
			log.Log.Error("webrtc.main.WriteToTrack(): failed to create AAC transcoder: " + err.Error())
		} else {
			aacTranscoder = t
			log.Log.Info("webrtc.main.WriteToTrack(): AAC transcoder created successfully")
			defer aacTranscoder.Close()
		}
	}

	if config.Capture.TranscodingWebRTC == "true" {
		log.Log.Info("webrtc.main.WriteToTrack(): transcoding config enabled")
	}

	// Initialize streaming state
	state := &streamState{
		lastKeepAlive: time.Now().Unix(),
		peerCount:     0,
	}

	defer func() {
		log.Log.Info(fmt.Sprintf("webrtc.main.WriteToTrack(): audio summary packets=%d aac_packets=%d sent=%d aac_no_output=%d aac_errors=%d peers=%d", state.audioPacketsSeen, state.aacPacketsSeen, state.audioSamplesSent, state.aacNoOutput, state.aacErrors, func() int {
			if audioBroadcaster == nil {
				return 0
			}
			return audioBroadcaster.PeerCount()
		}()))
		writeFinalSamples(state, videoBroadcaster, audioBroadcaster)
		log.Log.Info("webrtc.main.WriteToTrack(): stopped writing to track")
	}()

	var pkt packets.Packet
	var cursorError error

	for cursorError == nil {
		pkt, cursorError = livestreamCursor.ReadPacket()

		if cursorError != nil {
			break
		}

		// Update state from communication channels
		updateStreamState(communication, state)

		// Check if we should continue streaming
		if !shouldContinueStreaming(config, state) {
			state.start = false
			state.receivedKeyFrame = false
			continue
		}

		// Skip empty packets
		if len(pkt.Data) == 0 || pkt.Data == nil {
			state.receivedKeyFrame = false
			continue
		}

		// Keep live WebRTC close to realtime.
		// If audio+video load makes this consumer fall behind, skip old packets and
		// wait for a recent keyframe before resuming video.
		if shouldDropPacketForLatency(pkt) {
			if !state.catchingUp {
				log.Log.Warning("webrtc.main.WriteToTrack(): stream is lagging behind, dropping old packets until the next recent keyframe")
			}
			state.catchingUp = true
			state.start = false
			state.receivedKeyFrame = false
			state.lastAudioSample = nil
			state.lastVideoSample = nil
			continue
		}

		if state.catchingUp {
			if !(pkt.IsVideo && pkt.IsKeyFrame) {
				continue
			}
			state.catchingUp = false
			state.start = false
			state.receivedKeyFrame = false
			log.Log.Info("webrtc.main.WriteToTrack(): caught up with live stream at a recent keyframe")
		}

		// Wait for first keyframe before processing
		if !state.receivedKeyFrame {
			if pkt.IsKeyFrame {
				state.receivedKeyFrame = true
			} else {
				continue
			}
		}

		// Process video or audio packets
		if pkt.IsVideo {
			processVideoPacket(pkt, state, videoBroadcaster, config)
		} else if pkt.IsAudio {
			processAudioPacket(pkt, state, audioBroadcaster, aacTranscoder)
		}
	}
}
