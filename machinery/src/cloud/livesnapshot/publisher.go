// Package livesnapshot implements the agent-side producer for the live-view
// "preview" (SD) mode over HTTP.
//
// Historically the preview pipeline shipped each resized keyframe (a base64
// JPEG, often chunked) to viewers over the MQTT broker. MQTT is a control plane
// for small messages, so pushing ~1 image/second of base64 image data per
// watched camera congests the broker and delays genuine control traffic. This
// package moves those frames off MQTT: the agent POSTs the latest resized JPEG
// straight to hub-api over plain HTTPS (outbound only), and viewers fetch it
// back with their session token. Only the tiny "a viewer is watching" keepalive
// stays on MQTT.
//
// The wire contract (agent -> hub-api) deliberately mirrors the live HLS ingest
// and the existing storage-upload convention (X-Kerberos-Storage-Device plus the
// Hub public/private key auth headers). hub-api authenticates the agent and
// stores the frame in an ephemeral, short-TTL per-device slot which it serves
// straight back to authorized viewers; the frame never enters the vault or the
// recordings collection.
//
// Like live HLS segments, a preview frame is worthless once stale: a frame that
// fails to upload is superseded by the next one a second later, so the publisher
// is fire-and-forget and drops on failure (logged) rather than retrying.
package livesnapshot

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kerberos-io/agent/machinery/src/log"
)

const (
	// snapshotIngestPath is the hub-api endpoint that accepts the latest preview
	// frame and stores it in the device's ephemeral snapshot slot (mirrors the
	// /storage/live live-HLS ingest convention).
	snapshotIngestPath = "/storage/snapshot"

	contentTypeJPEG = "image/jpeg"

	// Header names for the snapshot ingest contract (shared with live HLS / storage).
	headerHubPublicKey  = "X-Kerberos-Hub-PublicKey"
	headerHubPrivateKey = "X-Kerberos-Hub-PrivateKey"
	headerHubRegion     = "X-Kerberos-Hub-Region"
	headerStorageDevice = "X-Kerberos-Storage-Device"

	// defaultPublishTimeout bounds a single snapshot upload. Preview frames are
	// produced roughly once a second from a single goroutine, so an upload that
	// cannot land in a few seconds is abandoned rather than allowed to back up the
	// preview loop behind a slow request.
	defaultPublishTimeout = 4 * time.Second
)

// PublisherConfig carries the hub endpoint and credentials needed to ship
// preview frames. It is populated from the agent's models.Config (the same
// HubURI/HubKey/HubPrivateKey used by recordings and live HLS).
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

// Publisher ships the latest preview frame to hub-api over plain HTTP POST.
//
// It is safe for sequential use from a single live-stream goroutine. PublishSnapshot
// is fire-and-forget: it returns an error for the caller to log, but the caller is
// expected to continue (drop-on-fail) rather than retry.
type Publisher struct {
	cfg    PublisherConfig
	client *http.Client
}

// NewPublisher builds a Publisher. The HTTP client strips the Hub credential
// headers on a cross-host redirect (net/http does this for standard auth headers
// but not custom-named ones), matching the recording/live-HLS upload clients.
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

// PublishSnapshot uploads a single resized preview frame (JPEG) as the device's
// latest snapshot. It overwrites whatever frame was there before, so viewers
// always fetch the most recent frame.
func (p *Publisher) PublishSnapshot(ctx context.Context, jpeg []byte) error {
	if p.cfg.HubURI == "" {
		return fmt.Errorf("livesnapshot: HubURI not configured")
	}
	if len(jpeg) == 0 {
		return fmt.Errorf("livesnapshot: empty snapshot body")
	}

	url := strings.TrimRight(p.cfg.HubURI, "/") + snapshotIngestPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jpeg))
	if err != nil {
		return fmt.Errorf("livesnapshot: build request: %w", err)
	}

	req.Header.Set("Content-Type", contentTypeJPEG)
	req.Header.Set(headerStorageDevice, p.cfg.DeviceKey)
	req.Header.Set(headerHubPublicKey, p.cfg.HubKey)
	req.Header.Set(headerHubPrivateKey, p.cfg.HubPrivateKey)
	req.Header.Set(headerHubRegion, p.cfg.Region)

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("livesnapshot: upload snapshot: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("livesnapshot: upload snapshot rejected: %s", resp.Status)
	}
	log.Log.Debug("livesnapshot.Publisher.PublishSnapshot(): shipped preview frame for device " + p.cfg.DeviceKey)
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
