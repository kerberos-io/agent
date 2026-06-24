// Package livehls implements the agent-side producer for live HLS streaming.
//
// It complements the recording pipeline: where recordings are muxed into one
// fragmented MP4 and uploaded resumably (TUS) when complete, live HLS ships a
// continuous series of small, independently-decodable CMAF segments to hub-api
// the instant each is produced, so a browser can play a near-live HLS stream
// without WebRTC/TURN (outbound HTTPS only).
//
// The wire contract (agent -> hub-api) intentionally mirrors the existing
// header-based storage convention (X-Kerberos-Storage-Device / -FileName, plus
// the Hub public/private key auth headers). hub-api authenticates the agent and
// stores each segment in an ephemeral, short-TTL live window keyed by
// {device}/{session}, which it serves straight back to the browser. The live
// window is deliberately kept out of the vault and the recordings collection;
// durable archival/DVR is a separate, later concern.
//
// Unlike recordings, live segments are NOT uploaded resumably: a 1-2s segment
// that fails to upload is stale by the time a retry would land, so the publisher
// is fire-and-forget and drops on failure (logged) rather than blocking the live
// pipeline behind a retry/handshake.
package livehls

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/video"
)

const (
	// liveIngestPath is the hub-api endpoint that accepts a single live segment
	// (or the init segment) and stores it in the ephemeral live window. hub-api
	// distinguishes init vs media segment and the object name via the
	// X-Kerberos-Live-* headers below, keeping a single route (mirrors the
	// existing /storage/upload convention).
	liveIngestPath = "/storage/live"

	// Object names within a session. The init segment (ftyp+moov) is uploaded
	// once per session; media segments are seg-<sequence>.m4s.
	initObjectName = "init.mp4"

	contentTypeInit    = "video/mp4"
	contentTypeSegment = "video/iso.segment"

	// Header names for the live ingest contract.
	headerHubPublicKey  = "X-Kerberos-Hub-PublicKey"
	headerHubPrivateKey = "X-Kerberos-Hub-PrivateKey"
	headerHubRegion     = "X-Kerberos-Hub-Region"
	headerStorageDevice = "X-Kerberos-Storage-Device"
	headerLiveSession   = "X-Kerberos-Live-Session"
	headerLiveName      = "X-Kerberos-Live-Name"
	headerLiveSequence  = "X-Kerberos-Live-Sequence"
	headerLiveDuration  = "X-Kerberos-Live-Duration"
	// Low-latency (LL-HLS) part headers. A part belongs to media segment
	// X-Kerberos-Live-Sequence and is the X-Kerberos-Live-Part-th chunk within it;
	// X-Kerberos-Live-Part-Independent flags a part that starts on a keyframe.
	headerLivePart            = "X-Kerberos-Live-Part"
	headerLivePartIndependent = "X-Kerberos-Live-Part-Independent"

	// defaultPublishTimeout bounds a single segment upload. A live segment that
	// cannot be delivered within roughly its own duration is stale, so the upload
	// is abandoned (dropped) rather than allowed to back up the pipeline.
	defaultPublishTimeout = 4 * time.Second
)

// PublisherConfig carries the hub endpoint and credentials needed to ship live
// segments. It is populated from the agent's models.Config (HubURI/HubKey/...).
type PublisherConfig struct {
	HubURI        string // base hub-api URL, e.g. https://api.hub.example.com
	HubKey        string // Hub public key (X-Kerberos-Hub-PublicKey)
	HubPrivateKey string // Hub private key (X-Kerberos-Hub-PrivateKey)
	Region        string // storage region (X-Kerberos-Hub-Region), may be empty
	DeviceKey     string // device/camera key (X-Kerberos-Storage-Device)

	// Timeout optionally overrides defaultPublishTimeout (used by tests).
	Timeout time.Duration
	// HTTPClient optionally injects a client (used by tests). When nil a
	// redirect-credential-stripping client is created.
	HTTPClient *http.Client
}

// Publisher ships init and media segments to hub-api over plain HTTP POST.
//
// It is safe for sequential use from a single live-stream goroutine. Methods are
// fire-and-forget: they return an error for the caller to log, but the caller is
// expected to continue (drop-on-fail) rather than retry.
type Publisher struct {
	cfg    PublisherConfig
	client *http.Client
}

// NewPublisher builds a Publisher. The HTTP client strips the Hub credential
// headers on a cross-host redirect (net/http does this for standard auth headers
// but not custom-named ones), matching the recording upload client.
func NewPublisher(cfg PublisherConfig) *Publisher {
	client := cfg.HTTPClient
	if client == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = defaultPublishTimeout
		}
		client = &http.Client{
			Timeout:       timeout,
			CheckRedirect: stripHubCredentialsOnCrossHostRedirect,
		}
	}
	return &Publisher{cfg: cfg, client: client}
}

// PublishInit uploads the session's init segment (ftyp+moov). It must be called
// (and succeed) before the player can use any media segment, so the caller
// should treat a failure here as "session not yet established" and retry on the
// next init opportunity rather than shipping media segments blindly.
func (p *Publisher) PublishInit(ctx context.Context, sessionID string, data []byte) error {
	return p.post(ctx, postParams{
		sessionID:   sessionID,
		name:        initObjectName,
		contentType: contentTypeInit,
		body:        data,
	})
}

// PublishSegment uploads one media segment (styp+moof+mdat). The segment's
// sequence number and duration travel in headers so hub-api can update the
// rolling playlist window without parsing the box structure.
func (p *Publisher) PublishSegment(ctx context.Context, sessionID string, seg video.LiveSegment) error {
	return p.post(ctx, postParams{
		sessionID:   sessionID,
		name:        fmt.Sprintf("seg-%d.m4s", seg.SequenceNumber),
		sequence:    seg.SequenceNumber,
		durationMs:  seg.DurationMs,
		hasSegment:  true,
		contentType: contentTypeSegment,
		body:        seg.Data,
	})
}

// PublishPart uploads one CMAF partial segment (LL-HLS). The part is named
// seg-<segment>.<part>.m4s and carries its segment sequence, part index,
// independence flag and duration in headers so hub-api can advertise it via
// #EXT-X-PART and reconstruct the full segment by concatenating its parts.
func (p *Publisher) PublishPart(ctx context.Context, sessionID string, part video.LivePart) error {
	return p.post(ctx, postParams{
		sessionID:   sessionID,
		name:        fmt.Sprintf("seg-%d.%d.m4s", part.SegmentSeq, part.PartIndex),
		sequence:    part.SegmentSeq,
		durationMs:  part.DurationMs,
		partIndex:   part.PartIndex,
		independent: part.Independent,
		hasPart:     true,
		contentType: contentTypeSegment,
		body:        part.Data,
	})
}

type postParams struct {
	sessionID   string
	name        string
	sequence    uint32
	durationMs  uint64
	hasSegment  bool
	partIndex   uint32
	independent bool
	hasPart     bool
	contentType string
	body        []byte
}

// post performs a single fire-and-forget upload to the live ingest endpoint.
func (p *Publisher) post(ctx context.Context, params postParams) error {
	if p.cfg.HubURI == "" {
		return fmt.Errorf("livehls: HubURI not configured")
	}
	if params.sessionID == "" {
		return fmt.Errorf("livehls: empty session id")
	}

	url := strings.TrimRight(p.cfg.HubURI, "/") + liveIngestPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(params.body))
	if err != nil {
		return fmt.Errorf("livehls: build request: %w", err)
	}

	req.Header.Set("Content-Type", params.contentType)
	req.Header.Set(headerStorageDevice, p.cfg.DeviceKey)
	req.Header.Set(headerLiveSession, params.sessionID)
	req.Header.Set(headerLiveName, params.name)
	if params.hasSegment || params.hasPart {
		req.Header.Set(headerLiveSequence, strconv.FormatUint(uint64(params.sequence), 10))
		req.Header.Set(headerLiveDuration, strconv.FormatUint(params.durationMs, 10))
	}
	if params.hasPart {
		req.Header.Set(headerLivePart, strconv.FormatUint(uint64(params.partIndex), 10))
		independent := "0"
		if params.independent {
			independent = "1"
		}
		req.Header.Set(headerLivePartIndependent, independent)
	}
	req.Header.Set(headerHubPublicKey, p.cfg.HubKey)
	req.Header.Set(headerHubPrivateKey, p.cfg.HubPrivateKey)
	req.Header.Set(headerHubRegion, p.cfg.Region)

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("livehls: upload %s: %w", params.name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("livehls: upload %s rejected: %s", params.name, resp.Status)
	}
	log.Log.Debug("livehls.Publisher.post(): shipped " + params.name + " for session " + params.sessionID)
	return nil
}

// stripHubCredentialsOnCrossHostRedirect removes the Hub credential headers when
// a redirect crosses to a different host. net/http strips standard sensitive
// headers on a cross-host redirect but not custom-named ones, so without this the
// Hub keys could leak to a redirect target.
func stripHubCredentialsOnCrossHostRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return nil
	}
	if req.URL.Host != via[0].URL.Host {
		req.Header.Del(headerHubPrivateKey)
		req.Header.Del(headerHubPublicKey)
	}
	return nil
}
