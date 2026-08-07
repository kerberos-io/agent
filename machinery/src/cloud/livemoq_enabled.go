//go:build moq

package cloud

import (
	"context"
	"fmt"
	"os"
	"strings"
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

// StartLiveStreamMoQ starts the publisher only in the dedicated MoQ build and
// only when explicitly enabled by the deployment.
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

	quality := os.Getenv("AGENT_LIVE_MOQ_QUALITY")
	if quality == "" {
		quality = models.StreamQualityAuto
	}
	useSub := models.SelectSubStreamForQuality(config, quality, subStreamEnabled)
	queue := communication.Queue
	sourceLabel := "main"
	if useSub && communication.SubQueue != nil {
		queue = communication.SubQueue
		sourceLabel = "sub"
	}
	if queue == nil {
		log.Log.Warning("cloud.StartLiveStreamMoQ(): selected packet queue is unavailable")
		return
	}

	relayURL := os.Getenv("AGENT_LIVE_MOQ_URL")
	if relayURL == "" {
		relayURL = defaultMoQRelayURL
	}
	publisherConfig := liveMoQConfig{
		relayURL:    relayURL,
		broadcast:   livemoq.BroadcastPath(os.Getenv("AGENT_LIVE_MOQ_BROADCAST_PREFIX"), config.Key),
		quality:     quality,
		sourceLabel: sourceLabel,
		queue:       queue,
	}

	ctx := context.Background()
	if communication.Context != nil {
		ctx = *communication.Context
	}
	go runLiveStreamMoQ(ctx, publisherConfig)
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

	cursor := config.queue.Latest()
	gate := livemoq.FrameGate{}
	var lastSlowWriteWarning time.Time
	for {
		packet, err := cursor.ReadPacket()
		if err != nil {
			return fmt.Errorf("read packet: %w", err)
		}
		if !packet.IsVideo || len(packet.Data) == 0 || !strings.EqualFold(packet.Codec, "H264") {
			continue
		}
		allowed, event := gate.Allow(packet.IsKeyFrame, packet.CurrentTime, time.Now(), maxMoQLivePacketAge)
		switch event {
		case livemoq.FrameGateEventStarted:
			log.Log.Info("cloud.publishLiveStreamMoQ(): first H.264 keyframe received; broadcast is live")
		case livemoq.FrameGateEventLagging:
			log.Log.Warning("cloud.publishLiveStreamMoQ(): stream is lagging; dropping packets until a recent keyframe")
		case livemoq.FrameGateEventRecovered:
			log.Log.Info("cloud.publishLiveStreamMoQ(): caught up with live stream at a recent keyframe")
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
				"cloud.publishLiveStreamMoQ(): WriteFrame blocked for %s (packet_age=%s keyframe=%t)",
				writeDuration.Round(time.Millisecond), packetAge.Round(time.Millisecond), packet.IsKeyFrame,
			))
			lastSlowWriteWarning = time.Now()
		}
	}
}
