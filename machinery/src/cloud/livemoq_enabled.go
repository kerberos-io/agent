//go:build moq

package cloud

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kerberos-io/agent/machinery/src/cloud/livemoq"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/packets"
	"github.com/moq-dev/moq-go/moq"
	log "github.com/sirupsen/logrus"
)

const (
	defaultMoQRelayURL      = "https://relay.uug.ai/anon"
	minMoQRetryDelay        = time.Second
	maxMoQRetryDelay        = 30 * time.Second
	defaultMoQLivePacketAge = 1500 * time.Millisecond
	minMoQLivePacketAge     = 250 * time.Millisecond
	maxMoQLivePacketAge     = 30 * time.Second
	defaultMoQWriteTimeout  = 5 * time.Second
	minMoQWriteTimeout      = time.Second
	maxMoQWriteTimeout      = time.Minute
	moQWriteWatchInterval   = 250 * time.Millisecond
	slowMoQWriteThreshold   = 100 * time.Millisecond
	moQWriteWarningInterval = 10 * time.Second
	duplicateKeyframeWindow = 500 * time.Millisecond
)

type liveMoQConfig struct {
	relayURL      string
	broadcast     string
	quality       string
	sourceLabel   string
	queue         *packets.Queue
	communication *models.Communication
	maxPacketAge  time.Duration
	writeTimeout  time.Duration
}

func (c liveMoQConfig) logEntry(event string) *log.Entry {
	return log.WithFields(log.Fields{
		"component": "moq",
		"event":     event,
		"quality":   c.quality,
		"stream":    c.sourceLabel,
	})
}

func moQRelayHost(relayURL string) string {
	parsed, err := url.Parse(relayURL)
	if err != nil {
		return ""
	}
	return parsed.Host
}

func (c liveMoQConfig) packetAgeLimit() time.Duration {
	if c.maxPacketAge > 0 {
		return c.maxPacketAge
	}
	return defaultMoQLivePacketAge
}

func (c liveMoQConfig) writeTimeoutLimit() time.Duration {
	if c.writeTimeout > 0 {
		return c.writeTimeout
	}
	return defaultMoQWriteTimeout
}

func boundedMoQDuration(name string, fallback, minimum, maximum time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}

	value, err := time.ParseDuration(raw)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"component":        "moq",
			"configured_value": raw,
			"effective_value":  fallback.String(),
			"event":            "duration_invalid",
			"variable":         name,
		}).Warn("Invalid MoQ duration; using default")
		return fallback
	}
	if value < minimum {
		log.WithFields(log.Fields{
			"component":        "moq",
			"configured_value": value.String(),
			"effective_value":  minimum.String(),
			"event":            "duration_clamped",
			"minimum":          minimum.String(),
			"variable":         name,
		}).Warn("MoQ duration is below the supported minimum")
		return minimum
	}
	if value > maximum {
		log.WithFields(log.Fields{
			"component":        "moq",
			"configured_value": value.String(),
			"effective_value":  maximum.String(),
			"event":            "duration_clamped",
			"maximum":          maximum.String(),
			"variable":         name,
		}).Warn("MoQ duration exceeds the supported maximum")
		return maximum
	}
	return value
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
func StartLiveStreamMoQ(
	ctx context.Context,
	configuration *models.Configuration,
	communication *models.Communication,
	subStreamEnabled bool,
	mainQueue *packets.Queue,
	subQueue *packets.Queue,
) {
	if os.Getenv("AGENT_LIVE_MOQ_ENABLED") != "true" {
		return
	}

	config := configuration.Config
	if config.Offline == "true" || config.Capture.Liveview == "false" {
		log.WithFields(log.Fields{
			"component": "moq",
			"event":     "publisher_disabled",
			"liveview":  config.Capture.Liveview,
			"offline":   config.Offline,
		}).Debug("MoQ publisher disabled by Agent configuration")
		return
	}
	if config.Key == "" {
		log.WithFields(log.Fields{
			"component": "moq",
			"event":     "publisher_configuration_invalid",
			"variable":  "AGENT_KEY",
		}).Warn("MoQ publisher requires an Agent key")
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
	maxPacketAge := boundedMoQDuration("AGENT_LIVE_MOQ_MAX_PACKET_AGE", defaultMoQLivePacketAge, minMoQLivePacketAge, maxMoQLivePacketAge)
	writeTimeout := boundedMoQDuration("AGENT_LIVE_MOQ_WRITE_TIMEOUT", defaultMoQWriteTimeout, minMoQWriteTimeout, maxMoQWriteTimeout)

	var publishers sync.WaitGroup
	for _, quality := range qualities {
		queue := mainQueue
		sourceLabel := "main"
		if models.SelectSubStreamForQuality(config, quality, subStreamEnabled) && subQueue != nil {
			queue = subQueue
			sourceLabel = "sub"
		}
		if queue == nil {
			log.WithFields(log.Fields{
				"component": "moq",
				"event":     "packet_queue_unavailable",
				"quality":   quality,
			}).Warn("MoQ packet queue is unavailable")
			continue
		}
		publisherConfig := liveMoQConfig{
			relayURL:      relayURL,
			broadcast:     livemoq.BroadcastPath(broadcastPrefix, config.Key, quality),
			quality:       quality,
			sourceLabel:   sourceLabel,
			queue:         queue,
			communication: communication,
			maxPacketAge:  maxPacketAge,
			writeTimeout:  writeTimeout,
		}
		publishers.Add(1)
		go func() {
			defer publishers.Done()
			runLiveStreamMoQ(ctx, publisherConfig)
		}()
	}
	publishers.Wait()
}

func runLiveStreamMoQ(ctx context.Context, config liveMoQConfig) {
	config.logEntry("publisher_started").Info("MoQ publisher started")
	config.logEntry("publisher_configuration").WithFields(log.Fields{
		"max_packet_age_ms": config.packetAgeLimit().Milliseconds(),
		"relay_host":        moQRelayHost(config.relayURL),
		"write_timeout_ms":  config.writeTimeoutLimit().Milliseconds(),
	}).Debug("MoQ publisher configuration")

	retryDelay := minMoQRetryDelay
	for ctx.Err() == nil {
		connectedAt := time.Now()
		err := publishLiveStreamMoQ(ctx, config)
		if ctx.Err() != nil {
			return
		}
		if config.communication != nil {
			config.communication.RecordMoQReconnect(config.quality)
		}
		config.logEntry("publisher_reconnecting").WithError(err).WithFields(log.Fields{
			"connected_duration_ms": time.Since(connectedAt).Milliseconds(),
			"retry_delay_ms":        retryDelay.Milliseconds(),
		}).Warn("MoQ publisher stopped; reconnecting")
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
	config.logEntry("relay_connected").WithField("relay_host", moQRelayHost(config.relayURL)).
		Info("MoQ relay connected")

	publisherCtx, cancelPublisher := context.WithCancel(ctx)
	defer cancelPublisher()
	sessionClosed := make(chan error, 1)
	sessionWatchDone := make(chan struct{})
	go func() {
		defer close(sessionWatchDone)
		sessionClosed <- client.Session().Closed(publisherCtx)
		cancelPublisher()
	}()

	broadcast, err := client.CreateBroadcast(config.broadcast)
	if err != nil {
		return fmt.Errorf("create broadcast: %w", err)
	}
	defer broadcast.Finish()

	stream, err := broadcast.PublishMedia("avc3", nil)
	if err != nil {
		return fmt.Errorf("create H.264 media stream: %w", err)
	}
	var finishStreamOnce sync.Once
	finishStream := func() {
		finishStreamOnce.Do(func() {
			_ = stream.Finish()
		})
	}
	defer finishStream()

	// Only upload while this tier is actually being watched. `publishing` starts
	// true so the track becomes discoverable on the relay even before the first
	// subscriber ever arrives; from the moment a viewer has attached once, the
	// subscriber watcher takes over and idles the tier again when everybody left.
	publishing := &atomic.Bool{}
	publishing.Store(true)
	subscriberWatchDone := make(chan struct{})
	go func() {
		defer close(subscriberWatchDone)
		watchLiveStreamMoQSubscribers(publisherCtx, stream, publishing, config)
	}()
	writeWatchdog := &livemoq.WriteWatchdog{}
	writeWatchDone := make(chan struct{})
	closePublisher := func() error {
		finishStream()
		return client.Close()
	}
	go func() {
		defer close(writeWatchDone)
		watchLiveStreamMoQWrites(publisherCtx, closePublisher, writeWatchdog, config)
	}()
	defer func() {
		cancelPublisher()
		<-writeWatchDone
		<-subscriberWatchDone
		<-sessionWatchDone
	}()

	cursor := config.queue.Latest()
	gate := livemoq.FrameGate{}
	deduplicator := livemoq.KeyframeDeduplicator{}
	var lastSlowWriteWarning time.Time
	var lastDuplicateKeyframeWarning time.Time
	idle := false
	for {
		select {
		case err := <-sessionClosed:
			return fmt.Errorf("relay session closed: %w", err)
		default:
		}

		packet, err := cursor.ReadPacketContext(publisherCtx)
		if err != nil {
			select {
			case sessionErr := <-sessionClosed:
				return fmt.Errorf("relay session closed: %w", sessionErr)
			default:
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("read packet: %w", err)
		}
		if !publishing.Load() {
			// Keep draining the cursor so we stay at the live edge, but publish
			// nothing. The gate is closed so the next viewer resumes on a keyframe.
			if !idle {
				gate.Reset()
				deduplicator.Reset()
				idle = true
			}
			continue
		}
		idle = false
		if !packet.IsVideo || len(packet.Data) == 0 || !strings.EqualFold(packet.Codec, "H264") {
			continue
		}
		allowed, event := gate.Allow(packet.IsKeyFrame, packet.CurrentTime, time.Now(), config.packetAgeLimit())
		switch event {
		case livemoq.FrameGateEventStarted:
			config.logEntry("broadcast_live").WithFields(log.Fields{
				"codec":        packet.Codec,
				"timestamp_ms": packet.Time,
			}).Info("MoQ broadcast is live")
		case livemoq.FrameGateEventLagging:
			config.logEntry("stream_lagging").WithFields(log.Fields{
				"max_packet_age_ms": config.packetAgeLimit().Milliseconds(),
				"timestamp_ms":      packet.Time,
			}).Warn("MoQ stream is lagging; dropping packets until a recent keyframe")
		case livemoq.FrameGateEventRecovered:
			config.logEntry("stream_recovered").WithField("timestamp_ms", packet.Time).
				Info("MoQ stream recovered at a recent keyframe")
		}
		if !allowed {
			continue
		}
		payload, normalizationStats, err := livemoq.NormalizeH264AccessUnitWithStats(packet.Data)
		if err != nil {
			return fmt.Errorf("normalize H.264 access unit: %w", err)
		}
		if normalizationStats.DuplicateIDRNALUs > 0 && time.Since(lastDuplicateKeyframeWarning) >= moQWriteWarningInterval {
			config.logEntry("duplicate_idr_removed").WithFields(log.Fields{
				"duplicate_idr_count": normalizationStats.DuplicateIDRNALUs,
				"timestamp_ms":        packet.Time,
			}).Warn("Removed duplicate IDR NAL units from MoQ keyframe")

			lastDuplicateKeyframeWarning = time.Now()
		}
		if packet.IsKeyFrame && deduplicator.IsDuplicate(packet.Time, packet.CurrentTime, payload, time.Now(), duplicateKeyframeWindow) {
			if time.Since(lastDuplicateKeyframeWarning) >= moQWriteWarningInterval {
				config.logEntry("duplicate_keyframe_dropped").WithField("timestamp_ms", packet.Time).
					Warn("Dropped duplicate MoQ keyframe")

				lastDuplicateKeyframeWarning = time.Now()
			}
			continue
		}
		frame := moq.Frame{
			Payload:     payload,
			TimestampUs: livemoq.TimestampUs(packet.Time),
		}
		writeStartedAt := time.Now()
		writeWatchdog.Begin(writeStartedAt)
		err = stream.WriteFrame(frame)
		writeWatchdog.End()
		if err != nil {
			return fmt.Errorf("write H.264 access unit: %w", err)
		}
		writeDuration := time.Since(writeStartedAt)
		if config.communication != nil {
			config.communication.RecordMoQWrite(config.quality, writeDuration, time.Now())
		}
		if writeDuration >= slowMoQWriteThreshold && time.Since(lastSlowWriteWarning) >= moQWriteWarningInterval {
			packetAge := time.Duration(0)
			if packet.CurrentTime > 0 {
				packetAge = time.Since(time.UnixMilli(packet.CurrentTime))
				if packetAge < 0 {
					packetAge = 0
				}
			}
			config.logEntry("slow_frame_write").WithFields(log.Fields{
				"duration_ms":   writeDuration.Milliseconds(),
				"keyframe":      packet.IsKeyFrame,
				"packet_age_ms": packetAge.Milliseconds(),
			}).Warn("MoQ frame write was slow")

			lastSlowWriteWarning = time.Now()
		}
	}
}

func watchLiveStreamMoQWrites(ctx context.Context, closeClient func() error, watchdog *livemoq.WriteWatchdog, config liveMoQConfig) {
	ticker := time.NewTicker(moQWriteWatchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			closeClient()
			return
		case now := <-ticker.C:
			writeDuration, active := watchdog.Elapsed(now)
			if !active || writeDuration < config.writeTimeoutLimit() {
				continue
			}
			config.logEntry("frame_write_timeout").WithFields(log.Fields{
				"duration_ms": writeDuration.Milliseconds(),
				"timeout_ms":  config.writeTimeoutLimit().Milliseconds(),
			}).Warn("MoQ frame write timed out; reconnecting relay client")

			if config.communication != nil {
				config.communication.RecordMoQWriteTimeout()
			}
			closeClient()
			return
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
			config.logEntry("subscriber_joined").Info("MoQ subscriber joined; resuming broadcast")
		}

		if err := stream.Unused(ctx); err != nil {
			publishing.Store(true)
			return
		}
		publishing.Store(false)
		config.logEntry("subscribers_idle").Info("MoQ broadcast idle; waiting for subscribers")
	}
}
