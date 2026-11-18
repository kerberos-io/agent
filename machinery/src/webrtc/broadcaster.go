package webrtc

import (
	"io"
	"sync"
	"time"

	"github.com/pion/rtcp"
	pionWebRTC "github.com/pion/webrtc/v4"
	pionMedia "github.com/pion/webrtc/v4/pkg/media"
	log "github.com/sirupsen/logrus"
)

const (
	// peerSampleBuffer controls how many samples can be buffered per peer before
	// dropping. Keeps slow peers from blocking the broadcaster.
	peerSampleBuffer        = 60
	keyframeRequestInterval = time.Second
)

type peerSample struct {
	pionMedia.Sample
	skippedDuration time.Duration
}

// peerTrack is a per-peer track with its own non-blocking sample channel.
type peerTrack struct {
	track           *pionWebRTC.TrackLocalStaticSample
	samples         chan peerSample
	done            chan struct{}
	connected       bool
	needsKeyframe   bool
	skippedDuration time.Duration
}

// TrackBroadcaster fans out media samples to multiple peer-specific tracks
// without blocking. Each peer gets its own TrackLocalStaticSample and a
// goroutine that drains samples independently, so a slow/congested peer
// cannot stall the others.
type TrackBroadcaster struct {
	mu              sync.RWMutex
	peers           map[string]*peerTrack
	mimeType        string
	id              string
	streamID        string
	requestKeyframe func() error
	requestTimer    *time.Timer
	requestInFlight bool
	lastRequest     time.Time
	requests        sync.WaitGroup
	closed          bool
}

// NewTrackBroadcaster creates a new broadcaster for either video or audio.
func NewTrackBroadcaster(mimeType string, id string, streamID string) *TrackBroadcaster {
	return &TrackBroadcaster{
		peers:    make(map[string]*peerTrack),
		mimeType: mimeType,
		id:       id,
		streamID: streamID,
	}
}

// AddPeer creates a new per-peer track and starts a writer goroutine.
// Returns the track to be added to the PeerConnection via AddTrack().
func (b *TrackBroadcaster) AddPeer(sessionKey string) (*pionWebRTC.TrackLocalStaticSample, error) {
	track, err := pionWebRTC.NewTrackLocalStaticSample(
		pionWebRTC.RTPCodecCapability{MimeType: b.mimeType},
		b.id,
		b.streamID,
	)
	if err != nil {
		return nil, err
	}

	pt := &peerTrack{
		track:         track,
		samples:       make(chan peerSample, peerSampleBuffer),
		done:          make(chan struct{}),
		connected:     b.mimeType != pionWebRTC.MimeTypeH264,
		needsKeyframe: b.mimeType == pionWebRTC.MimeTypeH264,
	}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, io.ErrClosedPipe
	}
	b.peers[sessionKey] = pt
	b.mu.Unlock()

	// Per-peer writer goroutine — drains samples independently.
	go func() {
		defer close(pt.done)
		for sample := range pt.samples {
			if err := writePeerSample(pt.track, sample); err != nil {
				if err == io.ErrClosedPipe {
					return
				}
				log.WithError(err).WithFields(log.Fields{
					"component":  "webrtc",
					"event":      "sample_write_failed",
					"session_id": sessionKey,
				}).Error("Failed to write WebRTC sample")
			}
		}
	}()

	log.WithFields(log.Fields{
		"component":  "webrtc",
		"event":      "peer_added",
		"session_id": sessionKey,
	}).Info("WebRTC peer added")
	return track, nil
}

func writePeerSample(track *pionWebRTC.TrackLocalStaticSample, sample peerSample) error {
	if sample.skippedDuration > 0 {
		// An empty sample advances Pion's RTP clock without sending a packet.
		if err := track.WriteSample(pionMedia.Sample{Duration: sample.skippedDuration}); err != nil {
			return err
		}
	}
	return track.WriteSample(sample.Sample)
}

// RemovePeer stops the writer goroutine and removes the peer.
func (b *TrackBroadcaster) RemovePeer(sessionKey string) {
	b.mu.Lock()
	pt, exists := b.peers[sessionKey]
	if exists {
		delete(b.peers, sessionKey)
	}
	b.mu.Unlock()

	if exists {
		close(pt.samples)
		<-pt.done // wait for writer goroutine to finish
		log.WithFields(log.Fields{
			"component":  "webrtc",
			"event":      "peer_removed",
			"session_id": sessionKey,
		}).Info("WebRTC peer removed")
	}
}

// WriteSample fans out a sample to all connected peers without blocking.
// If a peer's buffer is full (slow consumer), the sample is dropped for
// that peer only — other peers are unaffected.
func (b *TrackBroadcaster) WriteSample(sample pionMedia.Sample, isKeyframe bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}

	for sessionKey, pt := range b.peers {
		video := b.mimeType == pionWebRTC.MimeTypeH264
		if video && (!pt.connected || (pt.needsKeyframe && !isKeyframe)) {
			pt.skippedDuration += sample.Duration
			continue
		}
		select {
		case pt.samples <- peerSample{Sample: sample, skippedDuration: pt.skippedDuration}:
			pt.skippedDuration = 0
			if video && isKeyframe {
				pt.needsKeyframe = false
			}
		default:
			if video {
				pt.needsKeyframe = true
				drainPeerSamples(pt)
				if isKeyframe {
					pt.samples <- peerSample{Sample: sample, skippedDuration: pt.skippedDuration}
					pt.skippedDuration = 0
					pt.needsKeyframe = false
				} else {
					pt.skippedDuration += sample.Duration
				}
			}
			log.WithFields(log.Fields{
				"component":  "webrtc",
				"event":      "slow_peer_sample_dropped",
				"session_id": sessionKey,
			}).Warn("Dropping WebRTC sample for slow peer")
		}
	}
	b.scheduleKeyframeRequestLocked()
}

func drainPeerSamples(pt *peerTrack) {
	for {
		select {
		case sample := <-pt.samples:
			pt.skippedDuration += sample.skippedDuration + sample.Duration
		default:
			return
		}
	}
}

func (b *TrackBroadcaster) SetPeerConnected(sessionKey string, connected bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	pt, ok := b.peers[sessionKey]
	if !ok || b.mimeType != pionWebRTC.MimeTypeH264 || pt.connected == connected {
		return
	}
	pt.connected = connected
	pt.needsKeyframe = true
	drainPeerSamples(pt)
	b.scheduleKeyframeRequestLocked()
}

func (b *TrackBroadcaster) HandleRTCP(sessionKey string, data []byte) error {
	feedback, err := rtcp.Unmarshal(data)
	if err != nil {
		return err
	}
	for _, packet := range feedback {
		switch packet.(type) {
		case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
			b.mu.Lock()
			if pt, ok := b.peers[sessionKey]; ok && pt.connected && b.mimeType == pionWebRTC.MimeTypeH264 {
				pt.needsKeyframe = true
				drainPeerSamples(pt)
				b.scheduleKeyframeRequestLocked()
			}
			b.mu.Unlock()
			return nil
		}
	}
	return nil
}

func (b *TrackBroadcaster) waitingForKeyframeLocked() bool {
	for _, pt := range b.peers {
		if pt.connected && pt.needsKeyframe {
			return true
		}
	}
	return false
}

func (b *TrackBroadcaster) scheduleKeyframeRequestLocked() {
	if b.closed || b.requestKeyframe == nil || b.requestInFlight || !b.waitingForKeyframeLocked() {
		return
	}
	if b.requestTimer == nil {
		delay := time.Until(b.lastRequest.Add(keyframeRequestInterval))
		if delay < 0 {
			delay = 0
		}
		b.requestTimer = time.AfterFunc(delay, b.sendKeyframeRequest)
	}
}

func (b *TrackBroadcaster) sendKeyframeRequest() {
	b.mu.Lock()
	b.requestTimer = nil
	if b.closed || !b.waitingForKeyframeLocked() {
		b.mu.Unlock()
		return
	}
	b.lastRequest = time.Now()
	b.requestInFlight = true
	b.requests.Add(1)
	b.mu.Unlock()
	defer b.requests.Done()

	if err := b.requestKeyframe(); err != nil {
		log.WithError(err).WithFields(log.Fields{
			"component": "webrtc",
			"event":     "keyframe_request_failed",
		}).Warn("Camera keyframe request failed; waiting for a periodic IDR")
	} else {
		log.WithFields(log.Fields{
			"component": "webrtc",
			"event":     "keyframe_requested",
		}).Debug("Requested camera keyframe")
	}

	b.mu.Lock()
	b.requestInFlight = false
	b.scheduleKeyframeRequestLocked()
	b.mu.Unlock()
}

// PeerCount returns the current number of connected peers.
func (b *TrackBroadcaster) PeerCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.peers)
}

// Close removes all peers and stops all writer goroutines.
func (b *TrackBroadcaster) Close() {
	b.mu.Lock()
	b.closed = true
	if b.requestTimer != nil {
		b.requestTimer.Stop()
		b.requestTimer = nil
	}
	keys := make([]string, 0, len(b.peers))
	for k := range b.peers {
		keys = append(keys, k)
	}
	b.mu.Unlock()
	b.requests.Wait()

	for _, key := range keys {
		b.RemovePeer(key)
	}
}
