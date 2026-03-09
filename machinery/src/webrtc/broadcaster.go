package webrtc

import (
	"io"
	"sync"

	"github.com/kerberos-io/agent/machinery/src/log"
	pionWebRTC "github.com/pion/webrtc/v4"
	pionMedia "github.com/pion/webrtc/v4/pkg/media"
)

const (
	// peerSampleBuffer controls how many samples can be buffered per peer before
	// dropping. Keeps slow peers from blocking the broadcaster.
	peerSampleBuffer = 60
)

// peerTrack is a per-peer track with its own non-blocking sample channel.
type peerTrack struct {
	track   *pionWebRTC.TrackLocalStaticSample
	samples chan pionMedia.Sample
	done    chan struct{}
}

// TrackBroadcaster fans out media samples to multiple peer-specific tracks
// without blocking. Each peer gets its own TrackLocalStaticSample and a
// goroutine that drains samples independently, so a slow/congested peer
// cannot stall the others.
type TrackBroadcaster struct {
	mu       sync.RWMutex
	peers    map[string]*peerTrack
	mimeType string
	id       string
	streamID string
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
		track:   track,
		samples: make(chan pionMedia.Sample, peerSampleBuffer),
		done:    make(chan struct{}),
	}

	b.mu.Lock()
	b.peers[sessionKey] = pt
	b.mu.Unlock()

	// Per-peer writer goroutine — drains samples independently.
	go func() {
		defer close(pt.done)
		for sample := range pt.samples {
			if err := pt.track.WriteSample(sample); err != nil {
				if err == io.ErrClosedPipe {
					return
				}
				log.Log.Error("webrtc.broadcaster.peerWriter(): error writing sample for " + sessionKey + ": " + err.Error())
			}
		}
	}()

	log.Log.Info("webrtc.broadcaster.AddPeer(): added peer track for " + sessionKey)
	return track, nil
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
		log.Log.Info("webrtc.broadcaster.RemovePeer(): removed peer track for " + sessionKey)
	}
}

// WriteSample fans out a sample to all connected peers without blocking.
// If a peer's buffer is full (slow consumer), the sample is dropped for
// that peer only — other peers are unaffected.
func (b *TrackBroadcaster) WriteSample(sample pionMedia.Sample) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for sessionKey, pt := range b.peers {
		select {
		case pt.samples <- sample:
		default:
			log.Log.Warning("webrtc.broadcaster.WriteSample(): dropping sample for slow peer " + sessionKey)
		}
	}
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
	keys := make([]string, 0, len(b.peers))
	for k := range b.peers {
		keys = append(keys, k)
	}
	b.mu.Unlock()

	for _, key := range keys {
		b.RemovePeer(key)
	}
}
