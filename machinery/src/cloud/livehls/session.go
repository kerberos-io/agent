package livehls

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/packets"
	"github.com/kerberos-io/agent/machinery/src/video"
)

// DefaultTargetSegmentMs is the nominal live segment length. ~2s keeps standard
// HLS latency reasonable (a player typically buffers ~3 segments) while staying
// large enough that per-segment HTTP overhead is negligible.
const DefaultTargetSegmentMs = 2000

// Session ties a video.LiveSegmenter to a Publisher: it converts capture packets
// into CMAF segments and ships each one to hub-api. Exactly one init segment is
// delivered per session (re-attempted until it lands), after which media
// segments are published and the OnReady signal fires once so the control plane
// (MQTT) can tell viewers the live playlist exists.
//
// A Session is driven from a single goroutine (the live-stream loop); its methods
// are not safe for concurrent use except SessionID, which is immutable.
type Session struct {
	id        string
	publisher *Publisher
	segmenter *video.LiveSegmenter

	// newContext produces the per-upload context (timeout). Overridable in tests.
	newContext func() (context.Context, context.CancelFunc)

	mu            sync.Mutex
	initBytes     []byte
	initPublished bool
	// lastInitAt is when the init segment was last (re)uploaded. The init is
	// re-sent periodically so its short TTL in the hub live window never lapses
	// mid-session; see refreshInitIfStale.
	lastInitAt time.Time
	readyFired bool
	onReady    func(sessionID string)
}

// SessionOptions configures a live HLS session.
type SessionOptions struct {
	Codec           string   // "H264" or "H265"
	SPSNALUs        [][]byte // parameter sets (raw or Annex B)
	PPSNALUs        [][]byte //
	VPSNALUs        [][]byte // H.265 only
	Width           uint16   // encoded width (for the avcC fallback path)
	Height          uint16   // encoded height
	TargetSegmentMs uint64   // 0 => DefaultTargetSegmentMs
}

// NewSession builds a session with a fresh random id and wires the segmenter's
// init/segment callbacks to the publisher.
func NewSession(publisher *Publisher, opts SessionOptions) *Session {
	target := opts.TargetSegmentMs
	if target == 0 {
		target = DefaultTargetSegmentMs
	}
	seg := video.NewLiveSegmenter(opts.Codec, opts.SPSNALUs, opts.PPSNALUs, opts.VPSNALUs, target)
	seg.SetDimensions(opts.Width, opts.Height)

	s := &Session{
		id:        newSessionID(),
		publisher: publisher,
		segmenter: seg,
		newContext: func() (context.Context, context.CancelFunc) {
			return context.WithTimeout(context.Background(), defaultPublishTimeout)
		},
	}

	// The segmenter emits the init segment exactly once; capture it and try to
	// ship it. Failures here are non-fatal - publishInitIfNeeded re-attempts
	// before the next media segment so a transient hub hiccup at startup does not
	// permanently break the session.
	seg.OnInit = func(initBytes []byte) error {
		s.mu.Lock()
		s.initBytes = append([]byte(nil), initBytes...)
		s.mu.Unlock()
		s.publishInitIfNeeded()
		return nil
	}

	// Each completed media segment is shipped. We only publish a segment once the
	// init segment has landed (a media segment is useless without it), and we fire
	// OnReady after the first successfully shipped segment.
	seg.OnSegment = func(segment video.LiveSegment) error {
		if !s.publishInitIfNeeded() {
			log.Log.Warning("livehls.Session: dropping segment " +
				fmt.Sprintf("%d", segment.SequenceNumber) + " because init has not been delivered yet")
			return nil
		}
		ctx, cancel := s.newContext()
		defer cancel()
		if err := s.publisher.PublishSegment(ctx, s.id, segment); err != nil {
			log.Log.Warning("livehls.Session: " + err.Error())
			return nil
		}
		s.fireReadyOnce()
		// Keep the (write-once) init segment from ageing out of the live window
		// while the session is still producing media.
		s.refreshInitIfStale()
		return nil
	}

	return s
}

// SessionID returns the immutable session identifier used in object keys and the
// MQTT ready signal.
func (s *Session) SessionID() string { return s.id }

// IsReady reports whether the session has delivered its init segment and at
// least one media segment, i.e. the playlist hub-api serves is now playable. It
// lets the live-stream loop re-announce "receive-hls-ready" to viewers that join
// or hard-refresh after the initial one-shot signal (which they would otherwise
// never receive, leaving the stream blank until the session is recreated).
func (s *Session) IsReady() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readyFired
}

// SetOnReady registers a callback fired exactly once, after the first media
// segment has been successfully delivered. Used to publish the MQTT
// "receive-hls-ready" signal so viewers can load the playlist.
func (s *Session) SetOnReady(fn func(sessionID string)) {
	s.mu.Lock()
	s.onReady = fn
	s.mu.Unlock()
}

// WritePacket feeds one capture packet into the segmenter. Non-video packets are
// ignored (the spike is video-only). The decode timestamp is derived exactly as
// the recording muxer does: DTS = PTS - compositionOffset, with the composition
// offset forwarded for correct B-frame presentation order.
func (s *Session) WritePacket(pkt packets.Packet) error {
	if !pkt.IsVideo {
		return nil
	}
	pts := uint64(pkt.TimeLegacy.Milliseconds())
	compositionOffset := pkt.CompositionTime
	dts := pts
	if compositionOffset > 0 && uint64(compositionOffset) <= pts {
		dts = pts - uint64(compositionOffset)
	} else if compositionOffset < 0 || uint64(compositionOffset) > pts {
		// Guard against invalid offsets to avoid producing a CTS (DTS+CTO) jump.
		compositionOffset = 0
	}
	return s.segmenter.WriteSample(pkt.IsKeyFrame, pkt.Data, dts, int32(compositionOffset))
}

// Close flushes any buffered sample and ships the final segment.
func (s *Session) Close() error {
	return s.segmenter.Close()
}

// publishInitIfNeeded ensures the init segment has been delivered, attempting an
// upload if it has not. Returns true once init is known to be published.
func (s *Session) publishInitIfNeeded() bool {
	s.mu.Lock()
	if s.initPublished {
		s.mu.Unlock()
		return true
	}
	initBytes := s.initBytes
	s.mu.Unlock()

	if len(initBytes) == 0 {
		return false
	}

	ctx, cancel := s.newContext()
	defer cancel()
	if err := s.publisher.PublishInit(ctx, s.id, initBytes); err != nil {
		log.Log.Warning("livehls.Session: init upload failed, will retry: " + err.Error())
		return false
	}

	s.mu.Lock()
	s.initPublished = true
	s.lastInitAt = time.Now()
	s.mu.Unlock()
	log.Log.Info("livehls.Session: init segment delivered for session " + s.id)
	return true
}

// initRefreshInterval is how often the init segment is re-uploaded so its TTL in
// the hub-api live window never lapses mid-session. The init segment is otherwise
// written only once per session; because the live window expires objects after a
// short TTL (LiveSegmentTTLSeconds, 45s on the hub) the init would age out after
// ~1 minute and the playlist's #EXT-X-MAP would start 404ing, stalling playback.
// Re-uploading well inside that TTL keeps the init alive for the life of the
// session while still letting it expire naturally once the session ends.
const initRefreshInterval = 15 * time.Second

// refreshInitIfStale re-uploads the init segment if it has not been refreshed
// within initRefreshInterval, keeping its created_at (and thus its TTL) current
// for as long as the session is producing segments. It is a no-op until the init
// has first been published. Failures are non-fatal: the next segment retries.
func (s *Session) refreshInitIfStale() {
	s.mu.Lock()
	if !s.initPublished || time.Since(s.lastInitAt) < initRefreshInterval {
		s.mu.Unlock()
		return
	}
	initBytes := s.initBytes
	s.mu.Unlock()

	if len(initBytes) == 0 {
		return
	}

	ctx, cancel := s.newContext()
	defer cancel()
	if err := s.publisher.PublishInit(ctx, s.id, initBytes); err != nil {
		log.Log.Warning("livehls.Session: init refresh failed, will retry: " + err.Error())
		return
	}

	s.mu.Lock()
	s.lastInitAt = time.Now()
	s.mu.Unlock()
	log.Log.Debug("livehls.Session: refreshed init segment TTL for session " + s.id)
}

// fireReadyOnce invokes the OnReady callback the first time it is called.
func (s *Session) fireReadyOnce() {
	s.mu.Lock()
	if s.readyFired || s.onReady == nil {
		s.mu.Unlock()
		return
	}
	s.readyFired = true
	fn := s.onReady
	s.mu.Unlock()
	fn(s.id)
}

// newSessionID returns a short, unique, URL-safe session identifier of the form
// <unix-seconds>-<random-hex>.
func newSessionID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// rand.Read essentially never fails; fall back to a time-only id.
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%d-%s", time.Now().Unix(), hex.EncodeToString(b))
}
