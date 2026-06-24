package cloud

import (
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/kerberos-io/agent/machinery/src/capture"
	"github.com/kerberos-io/agent/machinery/src/cloud/livehls"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/packets"
)

// hlsViewerTimeoutSeconds is how long the agent keeps shipping live HLS segments
// after the last viewer keepalive. It is a few seconds longer than the segment
// duration so a viewer whose keepalive is briefly delayed does not cause the
// session to flap. When it lapses the session is torn down to stop wasting
// upload bandwidth when nobody is watching.
const hlsViewerTimeoutSeconds = 8

// hlsReadyReannounceSeconds throttles how often the agent re-announces an
// already-ready session over MQTT in response to viewer keepalives. The initial
// "receive-hls-ready" is a one-shot fired when the first segment lands; a viewer
// that connects or hard-refreshes after that (while the session is still alive)
// missed it, so we re-announce on subsequent keepalives. Viewers dedupe by
// session id, so a re-announce for a session they already play is a no-op. ~2s
// gets a refreshed viewer playing well within its connection timeout without
// spamming the control plane.
const hlsReadyReannounceSeconds = 2

// HandleLiveStreamHLS drives the live HLS producer. It mirrors HandleLiveStreamSD:
// it reads the camera's packet stream from a Latest() cursor, and while a viewer
// is active (kept alive via communication.HandleLiveHLS) it muxes the packets
// into CMAF segments and ships them to hub-api, which stores each segment in an
// ephemeral, short-TTL live window and serves the rolling playlist to viewers.
//
// A session is created lazily on the first keyframe seen while a viewer is active
// and torn down once viewers go away, so an idle camera produces no live traffic.
//
// By default (AGENT_LIVE_HLS_PREWARM unset or != "false") the agent instead keeps
// one long-lived session muxing continuously into a small in-memory ring buffer
// while idle (uploading nothing) and, the moment a viewer arrives, flushes the
// already-encoded init + most-recent segment(s) and starts uploading live. This
// trades a little idle CPU for a near-instant "requesting stream", so viewers no
// longer wait a full GOP for the first segment to be cut. Set
// AGENT_LIVE_HLS_PREWARM=false to fall back to the lazy on-demand path above.
func HandleLiveStreamHLS(livestreamCursor *packets.QueueCursor, configuration *models.Configuration, communication *models.Communication, mqttClient mqtt.Client, _ capture.RTSPClient) {

	log.Log.Debug("cloud.HandleLiveStreamHLS(): started")

	config := configuration.Config

	if config.Offline == "true" {
		log.Log.Debug("cloud.HandleLiveStreamHLS(): stopping as Offline is enabled.")
		return
	}
	if config.Capture.Liveview == "false" {
		log.Log.Debug("cloud.HandleLiveStreamHLS(): stopping as Liveview is disabled.")
		return
	}
	if config.HubURI == "" || config.HubKey == "" {
		log.Log.Debug("cloud.HandleLiveStreamHLS(): stopping as the Hub is not configured (HubURI/HubKey).")
		return
	}

	hubKey := config.HubKey
	deviceId := config.Key

	region := ""
	if config.S3 != nil {
		region = config.S3.Region
	}

	publisher := livehls.NewPublisher(livehls.PublisherConfig{
		HubURI:        config.HubURI,
		HubKey:        config.HubKey,
		HubPrivateKey: config.HubPrivateKey,
		Region:        region,
		DeviceKey:     deviceId,
	})

	// Encoded dimensions are only needed for the avcC fallback path (an SPS that
	// mp4ff's strict parser rejects); the main stream dimensions are a safe value.
	width := uint16(config.Capture.IPCamera.Width)
	height := uint16(config.Capture.IPCamera.Height)

	// prewarm keeps a single long-lived session muxing into an in-memory ring
	// buffer while idle and flushes it the instant a viewer arrives, eliminating
	// the per-request GOP wait. Enabled by default; set AGENT_LIVE_HLS_PREWARM=false
	// to fall back to the lazy on-demand path.
	prewarm := os.Getenv("AGENT_LIVE_HLS_PREWARM") != "false"
	if prewarm {
		log.Log.Info("cloud.HandleLiveStreamHLS(): live HLS prewarm ENABLED (set AGENT_LIVE_HLS_PREWARM=false to disable)")
	} else {
		log.Log.Info("cloud.HandleLiveStreamHLS(): live HLS prewarm DISABLED (AGENT_LIVE_HLS_PREWARM=false)")
	}

	// lowLatency enables LL-HLS: each segment is sliced into CMAF parts shipped the
	// instant they close and advertised via #EXT-X-PART, taking glass-to-glass HLS
	// latency from ~4-6s down to ~1-2s. Enabled by default; set
	// AGENT_LIVE_HLS_LOW_LATENCY=false to fall back to whole-segment HLS.
	partTargetMs := uint64(0)
	if os.Getenv("AGENT_LIVE_HLS_LOW_LATENCY") != "false" {
		partTargetMs = livehls.DefaultPartTargetMs
		log.Log.Info("cloud.HandleLiveStreamHLS(): live HLS low-latency (LL-HLS) ENABLED (set AGENT_LIVE_HLS_LOW_LATENCY=false to disable)")
	} else {
		log.Log.Info("cloud.HandleLiveStreamHLS(): live HLS low-latency (LL-HLS) DISABLED (AGENT_LIVE_HLS_LOW_LATENCY=false)")
	}

	var session *livehls.Session
	lastViewerRequest := int64(0)
	lastReadyAnnounce := int64(0)

	var cursorError error
	var pkt packets.Packet

	for cursorError == nil {
		pkt, cursorError = livestreamCursor.ReadPacket()

		now := time.Now().Unix()
		select {
		case <-communication.HandleLiveHLS:
			lastViewerRequest = now
			// A keepalive may come from a viewer that just connected or hard-
			// refreshed and therefore missed the one-shot readiness announcement
			// fired when this session's first segment landed. Re-announce (throttled)
			// so late/refreshed viewers learn the active session id; the frontend
			// dedupes by session id, so this is a no-op for viewers already playing.
			// UploadsActive() is always true for the on-demand path; for prewarm it
			// suppresses a stale re-announce while idle (the flush-on-arrival path
			// below announces once the buffer has actually been shipped).
			if session != nil && session.IsReady() && session.UploadsActive() && now-lastReadyAnnounce >= hlsReadyReannounceSeconds {
				publishHLSReady(configuration, mqttClient, hubKey, deviceId, session.SessionID())
				lastReadyAnnounce = now
			}
		default:
		}

		viewerActive := now-lastViewerRequest <= hlsViewerTimeoutSeconds

		if prewarm {
			// Keep one long-lived session muxing into the ring buffer. Create it on
			// the first keyframe (so the buffer opens on a random-access point) and
			// never tear it down for idleness; uploads, not muxing, are what we gate
			// on viewer presence.
			if session == nil {
				if len(pkt.Data) == 0 || !pkt.IsVideo || !pkt.IsKeyFrame {
					continue
				}
				session = livehls.NewSession(publisher, livehls.SessionOptions{
					Codec:          pkt.Codec,
					SPSNALUs:       config.Capture.IPCamera.SPSNALUs,
					PPSNALUs:       config.Capture.IPCamera.PPSNALUs,
					VPSNALUs:       config.Capture.IPCamera.VPSNALUs,
					Width:          width,
					Height:         height,
					PartTargetMs:   partTargetMs,
					StartBuffering: true,
				})
				session.SetOnReady(func(sessionID string) {
					log.Log.Info("cloud.HandleLiveStreamHLS(): live HLS session ready, announcing " + sessionID)
					publishHLSReady(configuration, mqttClient, hubKey, deviceId, sessionID)
					lastReadyAnnounce = time.Now().Unix()
				})
				log.Log.Info("cloud.HandleLiveStreamHLS(): prewarming live HLS session " + session.SessionID())
			}

			if viewerActive {
				// Activating flushes the cached init + buffered segment(s). onReady
				// announces the first-ever readiness; on a later re-activation it has
				// already fired, so announce here (throttled, so the first activation
				// does not double up) once the buffer has actually been shipped.
				if session.SetUploadsActive(true) && session.IsReady() && now-lastReadyAnnounce >= hlsReadyReannounceSeconds {
					publishHLSReady(configuration, mqttClient, hubKey, deviceId, session.SessionID())
					lastReadyAnnounce = now
				}
			} else {
				// No viewer: keep muxing into the buffer but stop uploading.
				session.SetUploadsActive(false)
			}

			if len(pkt.Data) > 0 && pkt.IsVideo {
				if err := session.WritePacket(pkt); err != nil {
					log.Log.Error("cloud.HandleLiveStreamHLS(): " + err.Error())
				}
			}
			continue
		}

		if !viewerActive {
			// No viewer: stop and discard the session so we stop shipping segments.
			if session != nil {
				_ = session.Close()
				log.Log.Info("cloud.HandleLiveStreamHLS(): no active viewers, stopped live HLS session " + session.SessionID())
				session = nil
			}
			continue
		}

		if len(pkt.Data) == 0 || !pkt.IsVideo {
			continue
		}

		// Start a session lazily, but only on a keyframe so the first segment opens
		// on a random-access point.
		if session == nil {
			if !pkt.IsKeyFrame {
				continue
			}
			session = livehls.NewSession(publisher, livehls.SessionOptions{
				Codec:        pkt.Codec,
				SPSNALUs:     config.Capture.IPCamera.SPSNALUs,
				PPSNALUs:     config.Capture.IPCamera.PPSNALUs,
				VPSNALUs:     config.Capture.IPCamera.VPSNALUs,
				Width:        width,
				Height:       height,
				PartTargetMs: partTargetMs,
			})
			session.SetOnReady(func(sessionID string) {
				log.Log.Info("cloud.HandleLiveStreamHLS(): live HLS session ready, announcing " + sessionID)
				publishHLSReady(configuration, mqttClient, hubKey, deviceId, sessionID)
				lastReadyAnnounce = time.Now().Unix()
			})
			log.Log.Info("cloud.HandleLiveStreamHLS(): started live HLS session " + session.SessionID())
		}

		if err := session.WritePacket(pkt); err != nil {
			log.Log.Error("cloud.HandleLiveStreamHLS(): " + err.Error())
		}
	}

	if session != nil {
		_ = session.Close()
	}
	log.Log.Debug("cloud.HandleLiveStreamHLS(): finished")
}

// publishHLSReady announces, over MQTT, that a live HLS session is available so
// viewers can load the rolling playlist hub-api serves for {device}/{session}.
func publishHLSReady(configuration *models.Configuration, mqttClient mqtt.Client, hubKey, deviceId, sessionID string) {
	valueMap := map[string]interface{}{
		"session": sessionID,
		"device":  deviceId,
	}
	message := models.Message{
		Payload: models.Payload{
			Action:   "receive-hls-ready",
			DeviceId: deviceId,
			Value:    valueMap,
		},
	}
	payload, err := models.PackageMQTTMessage(configuration, message)
	if err == nil {
		mqttClient.Publish("kerberos/hub/"+hubKey, 0, false, payload)
		log.Log.Info("cloud.HandleLiveStreamHLS(): announced live HLS session " + sessionID)
	} else {
		log.Log.Error("cloud.HandleLiveStreamHLS(): failed to package receive-hls-ready message: " + err.Error())
	}
}
