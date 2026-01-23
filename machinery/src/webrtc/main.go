package webrtc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	//"github.com/izern/go-fdkaac/fdkaac"
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
	keepAliveTimeout = 15 * time.Second
	defaultTimeout   = 10 * time.Second

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
	conn      *pionWebRTC.PeerConnection
	cancelCtx context.CancelFunc
	done      chan struct{}
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
func (cm *ConnectionManager) AddPeerConnection(sessionID string, wrapper *peerConnectionWrapper) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.peerConnections[sessionID] = wrapper
}

// RemovePeerConnection removes a peer connection from the manager
func (cm *ConnectionManager) RemovePeerConnection(sessionID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if wrapper, exists := cm.peerConnections[sessionID]; exists {
		if wrapper.cancelCtx != nil {
			wrapper.cancelCtx()
		}
		delete(cm.peerConnections, sessionID)
	}
}

// GetPeerConnectionCount returns the current count of active peer connections
func (cm *ConnectionManager) GetPeerConnectionCount() int64 {
	return atomic.LoadInt64(&cm.peerConnectionCount)
}

// IncrementPeerCount atomically increments the peer connection count
func (cm *ConnectionManager) IncrementPeerCount() int64 {
	return atomic.AddInt64(&cm.peerConnectionCount, 1)
}

// DecrementPeerCount atomically decrements the peer connection count
func (cm *ConnectionManager) DecrementPeerCount() int64 {
	return atomic.AddInt64(&cm.peerConnectionCount, -1)
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
	ch := globalConnectionManager.GetOrCreateCandidateChannel(key)

	log.Log.Info("webrtc.main.RegisterCandidates(): " + candidate.Candidate)
	select {
	case ch <- candidate.Candidate:
	default:
		log.Log.Info("webrtc.main.RegisterCandidates(): channel is full, dropping candidate")
	}
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

func InitializeWebRTCConnection(configuration *models.Configuration, communication *models.Communication, mqttClient mqtt.Client, videoTrack *pionWebRTC.TrackLocalStaticSample, audioTrack *pionWebRTC.TrackLocalStaticSample, handshake models.RequestHDStreamPayload) {

	config := configuration.Config
	deviceKey := config.Key
	stunServers := []string{config.STUNURI}
	turnServers := []string{config.TURNURI}
	turnServersUsername := config.TURNUsername
	turnServersCredential := config.TURNPassword

	// We create a channel which will hold the candidates for this session.
	sessionKey := config.Key + "/" + handshake.SessionID
	candidateChannel := globalConnectionManager.GetOrCreateCandidateChannel(sessionKey)

	// Set variables
	hubKey := handshake.HubKey
	sessionDescription := handshake.SessionDescription

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
				conn:      peerConnection,
				cancelCtx: cancel,
				done:      make(chan struct{}),
			}

			var videoSender *pionWebRTC.RTPSender = nil
			if videoTrack != nil {
				if videoSender, err = peerConnection.AddTrack(videoTrack); err != nil {
					log.Log.Error("webrtc.main.InitializeWebRTCConnection(): error adding video track: " + err.Error())
					cancel()
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

			var audioSender *pionWebRTC.RTPSender = nil
			if audioTrack != nil {
				if audioSender, err = peerConnection.AddTrack(audioTrack); err != nil {
					log.Log.Error("webrtc.main.InitializeWebRTCConnection(): error adding audio track: " + err.Error())
					cancel()
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

			peerConnection.OnConnectionStateChange(func(connectionState pionWebRTC.PeerConnectionState) {
				log.Log.Info("webrtc.main.InitializeWebRTCConnection(): connection state changed to: " + connectionState.String())

				switch connectionState {
				case pionWebRTC.PeerConnectionStateDisconnected, pionWebRTC.PeerConnectionStateClosed:
					count := globalConnectionManager.DecrementPeerCount()
					log.Log.Info("webrtc.main.InitializeWebRTCConnection(): Peer disconnected. Active peers: " + string(rune(count)))

					// Clean up resources
					globalConnectionManager.CloseCandidateChannel(sessionKey)

					if err := peerConnection.Close(); err != nil {
						log.Log.Error("webrtc.main.InitializeWebRTCConnection(): error closing peer connection: " + err.Error())
					}

					globalConnectionManager.RemovePeerConnection(handshake.SessionID)
					close(wrapper.done)

				case pionWebRTC.PeerConnectionStateConnected:
					count := globalConnectionManager.IncrementPeerCount()
					log.Log.Info("webrtc.main.InitializeWebRTCConnection(): Peer connected. Active peers: " + string(rune(count)))

				case pionWebRTC.PeerConnectionStateFailed:
					log.Log.Info("webrtc.main.InitializeWebRTCConnection(): ICE connection failed")
				}
			})

			go func() {
				defer func() {
					log.Log.Info("webrtc.main.InitializeWebRTCConnection(): candidate processor stopped for session: " + handshake.SessionID)
				}()

				// Iterate over the candidates and send them to the remote client
				for {
					select {
					case <-ctx.Done():
						return
					case candidate, ok := <-candidateChannel:
						if !ok {
							return
						}
						log.Log.Info("webrtc.main.InitializeWebRTCConnection(): Received candidate from channel: " + candidate)
						if candidateErr := peerConnection.AddICECandidate(pionWebRTC.ICECandidateInit{Candidate: candidate}); candidateErr != nil {
							log.Log.Error("webrtc.main.InitializeWebRTCConnection(): error adding candidate: " + candidateErr.Error())
						}
					}
				}
			}()

			offer := w.CreateOffer(sd)
			if err = peerConnection.SetRemoteDescription(offer); err != nil {
				log.Log.Error("webrtc.main.InitializeWebRTCConnection(): something went wrong while setting remote description: " + err.Error())
			}

			answer, err := peerConnection.CreateAnswer(nil)
			if err != nil {
				log.Log.Error("webrtc.main.InitializeWebRTCConnection(): something went wrong while creating answer: " + err.Error())
			} else if err = peerConnection.SetLocalDescription(answer); err != nil {
				log.Log.Error("webrtc.main.InitializeWebRTCConnection(): something went wrong while setting local description: " + err.Error())
			}

			// When an ICE candidate is available send to the other peer using the signaling server (MQTT).
			// The other peer will add this candidate by calling AddICECandidate
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
					// SDP is not needed to be send..
					//valueMap["sdp"] = []byte(base64.StdEncoding.EncodeToString([]byte(answer.SDP)))
					valueMap["session_id"] = handshake.SessionID
					log.Log.Info("webrtc.main.InitializeWebRTCConnection(): sending " + candidateType + " candidate to hub")
				} else {
					log.Log.Error("webrtc.main.InitializeWebRTCConnection(): failed to marshal candidate: " + err.Error())
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
					token := mqttClient.Publish("kerberos/hub/"+hubKey, 2, false, payload)
					token.Wait()
				} else {
					log.Log.Info("webrtc.main.InitializeWebRTCConnection(): while packaging mqtt message: " + err.Error())
				}
			})

			// Store peer connection in manager
			globalConnectionManager.AddPeerConnection(handshake.SessionID, wrapper)

			if err == nil {
				//  Create a config map
				valueMap := make(map[string]interface{})
				valueMap["sdp"] = []byte(base64.StdEncoding.EncodeToString([]byte(answer.SDP)))
				valueMap["session_id"] = handshake.SessionID
				log.Log.Info("webrtc.main.InitializeWebRTCConnection(): Send SDP answer")

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
					log.Log.Info("webrtc.main.InitializeWebRTCConnection(): while packaging mqtt message: " + err.Error())
				}
			}
		}
	} else {
		log.Log.Error("Initializwebrtc.main.InitializeWebRTCConnection()eWebRTCConnection: NewPeerConnection failed: " + err.Error())
	}
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
	for _, stream := range streams {
		if stream.Name == "OPUS" {
			mimeType = pionWebRTC.MimeTypeOpus
		} else if stream.Name == "PCM_MULAW" {
			mimeType = pionWebRTC.MimeTypePCMU
		} else if stream.Name == "PCM_ALAW" {
			mimeType = pionWebRTC.MimeTypePCMA
		}
	}
	if mimeType == "" {
		log.Log.Error("webrtc.main.NewAudioTrack(): no supported audio codec found")
		return nil
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
	receivedKeyFrame bool
	lastAudioSample  *pionMedia.Sample
	lastVideoSample  *pionMedia.Sample
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
func writeFinalSamples(state *streamState, videoTrack, audioTrack *pionWebRTC.TrackLocalStaticSample) {
	if state.lastVideoSample != nil && videoTrack != nil {
		if err := videoTrack.WriteSample(*state.lastVideoSample); err != nil && err != io.ErrClosedPipe {
			log.Log.Error("webrtc.main.writeFinalSamples(): error writing final video sample: " + err.Error())
		}
	}

	if state.lastAudioSample != nil && audioTrack != nil {
		if err := audioTrack.WriteSample(*state.lastAudioSample); err != nil && err != io.ErrClosedPipe {
			log.Log.Error("webrtc.main.writeFinalSamples(): error writing final audio sample: " + err.Error())
		}
	}
}

// processVideoPacket processes a video packet and writes samples to the track
func processVideoPacket(pkt packets.Packet, state *streamState, videoTrack *pionWebRTC.TrackLocalStaticSample, config models.Config) {
	if videoTrack == nil {
		return
	}

	// Start at the first keyframe
	if pkt.IsKeyFrame {
		state.start = true
	}

	if !state.start {
		return
	}

	sample := pionMedia.Sample{Data: pkt.Data, PacketTimestamp: uint32(pkt.Time)}

	if config.Capture.ForwardWebRTC == "true" {
		// Remote forwarding not yet implemented
		log.Log.Debug("webrtc.main.processVideoPacket(): remote forwarding not implemented")
		return
	}

	if state.lastVideoSample != nil {
		duration := sample.PacketTimestamp - state.lastVideoSample.PacketTimestamp
		state.lastVideoSample.Duration = time.Duration(duration) * time.Millisecond

		if err := videoTrack.WriteSample(*state.lastVideoSample); err != nil && err != io.ErrClosedPipe {
			log.Log.Error("webrtc.main.processVideoPacket(): error writing video sample: " + err.Error())
		}
	}

	state.lastVideoSample = &sample
}

// processAudioPacket processes an audio packet and writes samples to the track
func processAudioPacket(pkt packets.Packet, state *streamState, audioTrack *pionWebRTC.TrackLocalStaticSample, hasAAC bool) {
	if audioTrack == nil {
		return
	}

	if hasAAC {
		// AAC transcoding not yet implemented
		// TODO: Implement AAC to PCM_MULAW transcoding
		return
	}

	sample := pionMedia.Sample{Data: pkt.Data, PacketTimestamp: uint32(pkt.Time)}

	if state.lastAudioSample != nil {
		duration := sample.PacketTimestamp - state.lastAudioSample.PacketTimestamp
		state.lastAudioSample.Duration = time.Duration(duration) * time.Millisecond

		if err := audioTrack.WriteSample(*state.lastAudioSample); err != nil && err != io.ErrClosedPipe {
			log.Log.Error("webrtc.main.processAudioPacket(): error writing audio sample: " + err.Error())
		}
	}

	state.lastAudioSample = &sample
}

func WriteToTrack(livestreamCursor *packets.QueueCursor, configuration *models.Configuration, communication *models.Communication, mqttClient mqtt.Client, videoTrack *pionWebRTC.TrackLocalStaticSample, audioTrack *pionWebRTC.TrackLocalStaticSample, rtspClient capture.RTSPClient) {

	config := configuration.Config

	// Check if at least one track is available
	if videoTrack == nil && audioTrack == nil {
		log.Log.Error("webrtc.main.WriteToTrack(): both video and audio tracks are nil, cannot proceed")
		return
	}

	// Detect available codecs
	codecs := detectCodecs(rtspClient)

	if !codecs.hasValidCodecs() {
		log.Log.Error("webrtc.main.WriteToTrack(): no valid video or audio codec found")
		return
	}

	if config.Capture.TranscodingWebRTC == "true" {
		log.Log.Info("webrtc.main.WriteToTrack(): transcoding enabled but not yet implemented")
	}

	// Initialize streaming state
	state := &streamState{
		lastKeepAlive: time.Now().Unix(),
		peerCount:     0,
	}

	defer func() {
		writeFinalSamples(state, videoTrack, audioTrack)
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
			processVideoPacket(pkt, state, videoTrack, config)
		} else if pkt.IsAudio {
			processAudioPacket(pkt, state, audioTrack, codecs.hasAAC)
		}
	}
}
