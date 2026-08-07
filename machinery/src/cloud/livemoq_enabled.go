//go:build moq

package cloud

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kerberos-io/agent/machinery/src/cloud/livemoq"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/packets"
	"github.com/moq-dev/moq-go/moq"
)

const (
	defaultMoQRelayURL      = "https://relay.uug.ai/anon"
	minMoQRetryDelay        = time.Second
	maxMoQRetryDelay        = 30 * time.Second
	maxMoQLivePacketAge     = 1500 * time.Millisecond
	slowMoQWriteThreshold   = 100 * time.Millisecond
	moQWriteWarningInterval = 10 * time.Second
)

type liveMoQConfig struct {
	relayURL    string
	broadcast   string
	quality     string
	sourceLabel string
	queue       *packets.Queue
}

// label identifies the tier in log lines, since one Agent runs a publisher per
// quality tier.
func (c liveMoQConfig) label() string {
	return c.quality + " (" + c.sourceLabel + " stream)"
}

// StartLiveStreamMoQ starts the publisher only in the dedicated MoQ build and
// only when explicitly enabled by the deployment.
//
// Unlike WebRTC and HLS — where a viewer negotiates a session with the Agent and
// can therefore ask for another quality on the fly — MoQ viewers subscribe to a
// relay and never talk to the Agent. The quality selector is honoured by
// publishing each tier as its OWN broadcast (see livemoq.BroadcastPath): the
// high tier from the camera's highest-resolution stream and the low tier from
// its sub stream, so switching quality in the frontend is a resubscribe to the
// other path. Each tier only uploads while it actually has subscribers, so the
// second broadcast is close to free when nobody watches it.
func StartLiveStreamMoQ(configuration *models.Configuration, communication *models.Communication, subStreamEnabled bool) {
	if os.Getenv("AGENT_LIVE_MOQ_ENABLED") != "true" {
		return
	}

	config := configuration.Config
	if config.Offline == "true" || config.Capture.Liveview == "false" {
		log.Log.Info("cloud.StartLiveStreamMoQ(): disabled by Agent live-view configuration")
		return
	}
	if config.Key == "" {
		log.Log.Warning("cloud.StartLiveStreamMoQ(): AGENT_KEY is required")
		return
	}

	// Both tiers are published by default. AGENT_LIVE_MOQ_QUALITY pins the Agent
	// to a single tier for deployments that must never publish the other one
	// (viewers asking for the pinned-away tier then find no broadcast).
	qualities := []string{models.StreamQualityHigh, models.StreamQualityLow}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AGENT_LIVE_MOQ_QUALITY"))) {
	case models.StreamQualityHigh:
		qualities = []string{models.StreamQualityHigh}
	case models.StreamQualityLow:
		qualities = []string{models.StreamQualityLow}
	}

	relayURL := os.Getenv("AGENT_LIVE_MOQ_URL")
	if relayURL == "" {
		relayURL = defaultMoQRelayURL
	}
	broadcastPrefix := os.Getenv("AGENT_LIVE_MOQ_BROADCAST_PREFIX")

	ctx := context.Background()
	if communication.Context != nil {
		ctx = *communication.Context
	}

	for _, quality := range qualities {
		queue := communication.Queue
		sourceLabel := "main"
		if models.SelectSubStreamForQuality(config, quality, subStreamEnabled) && communication.SubQueue != nil {
			queue = communication.SubQueue
			sourceLabel = "sub"
		}
		if queue == nil {
			log.Log.Warning("cloud.StartLiveStreamMoQ(): packet queue for the " + quality + " tier is unavailable")
			continue
		}
		go runLiveStreamMoQ(ctx, liveMoQConfig{
			relayURL:    relayURL,
			broadcast:   livemoq.BroadcastPath(broadcastPrefix, config.Key, quality),
			quality:     quality,
			sourceLabel: sourceLabel,
			queue:       queue,
		})
	}
}

func runLiveStreamMoQ(ctx context.Context, config liveMoQConfig) {
	log.Log.Info(fmt.Sprintf(
		"cloud.runLiveStreamMoQ(): publishing %s stream (quality=%s) to %s/%s",
		config.sourceLabel, config.quality, strings.TrimRight(config.relayURL, "/"), config.broadcast,
	))

	retryDelay := minMoQRetryDelay
	for ctx.Err() == nil {
		connectedAt := time.Now()
		err := publishLiveStreamMoQ(ctx, config)
		if ctx.Err() != nil {
			return
		}
		log.Log.Warning("cloud.runLiveStreamMoQ(): publisher stopped: " + err.Error())
		if time.Since(connectedAt) >= time.Minute {
			retryDelay = minMoQRetryDelay
		}

		timer := time.NewTimer(retryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if retryDelay < maxMoQRetryDelay {
			retryDelay *= 2
			if retryDelay > maxMoQRetryDelay {
				retryDelay = maxMoQRetryDelay
			}
		}
	}
}

func publishLiveStreamMoQ(ctx context.Context, config liveMoQConfig) error {
	client, err := moq.Dial(ctx, config.relayURL)
	if err != nil {
		return fmt.Errorf("connect to relay: %w", err)
	}
	defer client.Close()

	broadcast, err := client.CreateBroadcast(config.broadcast)
	if err != nil {
		return fmt.Errorf("create broadcast: %w", err)
	}
	defer broadcast.Finish()

	stream, err := broadcast.PublishMedia("avc3", nil)
	if err != nil {
		return fmt.Errorf("create H.264 media stream: %w", err)
	}
	defer stream.Finish()

	// Only upload while this tier is actually being watched. `publishing` starts
	// true so the track becomes discoverable on the relay even before the first
	// subscriber ever arrives; from the moment a viewer has attached once, the
	// subscriber watcher takes over and idles the tier again when everybody left.
	watchCtx, cancelWatch := context.WithCancel(ctx)
	defer cancelWatch()
	publishing := &atomic.Bool{}
	publishing.Store(true)
	go watchLiveStreamMoQSubscribers(watchCtx, stream, publishing, config)

	cursor := config.queue.Latest()
	gate := livemoq.FrameGate{}
	var lastSlowWriteWarning time.Time
	idle := false
	for {
		packet, err := cursor.ReadPacket()
		if err != nil {
			return fmt.Errorf("read packet: %w", err)
		}
		if !publishing.Load() {
			// Keep draining the cursor so we stay at the live edge, but publish
			// nothing. The gate is closed so the next viewer resumes on a keyframe.
			if !idle {
				gate.Reset()
				idle = true
			}
			continue
		}
		idle = false
		if !packet.IsVideo || len(packet.Data) == 0 || !strings.EqualFold(packet.Codec, "H264") {
			continue
		}
		allowed, event := gate.Allow(packet.IsKeyFrame, packet.CurrentTime, time.Now(), maxMoQLivePacketAge)
		switch event {
		case livemoq.FrameGateEventStarted:
			log.Log.Info("cloud.publishLiveStreamMoQ(): first H.264 keyframe received; " + config.label() + " broadcast is live")
		case livemoq.FrameGateEventLagging:
			log.Log.Warning("cloud.publishLiveStreamMoQ(): " + config.label() + " stream is lagging; dropping packets until a recent keyframe")
		case livemoq.FrameGateEventRecovered:
			log.Log.Info("cloud.publishLiveStreamMoQ(): caught up with the " + config.label() + " live stream at a recent keyframe")
		}
		if !allowed {
			continue
		}
		payload, err := livemoq.NormalizeH264AccessUnit(packet.Data)
		if err != nil {
			return fmt.Errorf("normalize H.264 access unit: %w", err)
		}
		frame := moq.Frame{
			Payload:     payload,
			TimestampUs: livemoq.TimestampUs(packet.Time),
		}
		writeStartedAt := time.Now()
		if err := stream.WriteFrame(frame); err != nil {
			return fmt.Errorf("write H.264 access unit: %w", err)
		}
		writeDuration := time.Since(writeStartedAt)
		if writeDuration >= slowMoQWriteThreshold && time.Since(lastSlowWriteWarning) >= moQWriteWarningInterval {
			packetAge := time.Duration(0)
			if packet.CurrentTime > 0 {
				packetAge = time.Since(time.UnixMilli(packet.CurrentTime))
				if packetAge < 0 {
					packetAge = 0
				}
			}
			log.Log.Warning(fmt.Sprintf(
				"cloud.publishLiveStreamMoQ(): %s WriteFrame blocked for %s (packet_age=%s keyframe=%t)",
				config.label(), writeDuration.Round(time.Millisecond), packetAge.Round(time.Millisecond), packet.IsKeyFrame,
			))
			lastSlowWriteWarning = time.Now()
		}
	}
}

// watchLiveStreamMoQSubscribers flips the publisher between uploading and idling
// as viewers subscribe to and leave this tier's broadcast. Used and Unused both
// block, so they are followed from their own goroutine.
//
// It deliberately never turns publishing off before the first subscriber has
// been observed: the relay catalog is only complete once media has flowed, so
// going idle up front could keep the tier undiscoverable. On any error it fails
// open (keeps publishing) — a stalled watcher must never take the live view down.
func watchLiveStreamMoQSubscribers(ctx context.Context, stream *moq.MediaProducer, publishing *atomic.Bool, config liveMoQConfig) {
	for ctx.Err() == nil {
		if err := stream.Used(ctx); err != nil {
			publishing.Store(true)
			return
		}
		if publishing.CompareAndSwap(false, true) {
			log.Log.Info("cloud.watchLiveStreamMoQSubscribers(): viewer subscribed, resuming the " + config.label() + " broadcast")
		}

		if err := stream.Unused(ctx); err != nil {
			publishing.Store(true)
			return
		}
		publishing.Store(false)
		log.Log.Info("cloud.watchLiveStreamMoQSubscribers(): no viewers left, idling the " + config.label() + " broadcast")
	}
}
