package webrtc

import (
	"errors"
	"testing"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	pionWebRTC "github.com/pion/webrtc/v4"
	pionMedia "github.com/pion/webrtc/v4/pkg/media"
)

type clockTestWriter struct {
	timestamps []uint32
}

func (w *clockTestWriter) WriteRTP(header *rtp.Header, payload []byte) (int, error) {
	w.timestamps = append(w.timestamps, header.Timestamp)
	return len(payload), nil
}

func (*clockTestWriter) Write([]byte) (int, error) {
	return 0, errors.New("unexpected raw RTP write")
}

type clockTestContext struct {
	writer *clockTestWriter
}

func (clockTestContext) ID() string                                  { return "clock-test" }
func (clockTestContext) SSRC() pionWebRTC.SSRC                       { return 123 }
func (clockTestContext) SSRCRetransmission() pionWebRTC.SSRC         { return 0 }
func (clockTestContext) SSRCForwardErrorCorrection() pionWebRTC.SSRC { return 0 }
func (clockTestContext) RTCPReader() interceptor.RTCPReader          { return nil }
func (clockTestContext) HeaderExtensions() []pionWebRTC.RTPHeaderExtensionParameter {
	return nil
}
func (c clockTestContext) WriteStream() pionWebRTC.TrackLocalWriter { return c.writer }
func (clockTestContext) CodecParameters() []pionWebRTC.RTPCodecParameters {
	return []pionWebRTC.RTPCodecParameters{{
		RTPCodecCapability: pionWebRTC.RTPCodecCapability{MimeType: pionWebRTC.MimeTypeH264, ClockRate: 90000},
		PayloadType:        96,
	}}
}

func clockTestPeer(t *testing.T) (*TrackBroadcaster, *peerTrack, *clockTestWriter) {
	t.Helper()
	b, pt := testVideoBroadcaster(t)
	pt.samples = make(chan peerSample, 2)
	track, err := pionWebRTC.NewTrackLocalStaticSample(
		pionWebRTC.RTPCodecCapability{MimeType: pionWebRTC.MimeTypeH264},
		"video", "clock-test", pionWebRTC.WithRTPTimestamp(1000),
	)
	if err != nil {
		t.Fatal(err)
	}
	writer := &clockTestWriter{}
	if _, err := track.Bind(clockTestContext{writer: writer}); err != nil {
		t.Fatal(err)
	}
	pt.track = track
	b.SetPeerConnected("viewer", true)
	return b, pt, writer
}

func clockVideoSample(duration time.Duration) pionMedia.Sample {
	return pionMedia.Sample{
		Data:     []byte{0, 0, 0, 1, 0x65, 0x88, 0x84},
		Duration: duration,
	}
}

func writeClockSample(t *testing.T, pt *peerTrack) {
	t.Helper()
	select {
	case sample := <-pt.samples:
		if err := writePeerSample(pt.track, sample); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("expected queued video sample")
	}
}

func TestVideoRecoveryPreservesRTPClock(t *testing.T) {
	for _, scenario := range []string{"PLI", "FIR", "reconnect", "overflow"} {
		t.Run(scenario, func(t *testing.T) {
			b, pt, writer := clockTestPeer(t)
			b.WriteSample(clockVideoSample(40*time.Millisecond), true)
			writeClockSample(t, pt)
			b.WriteSample(clockVideoSample(70*time.Millisecond), false)

			switch scenario {
			case "PLI", "FIR":
				var feedback rtcp.Packet = &rtcp.PictureLossIndication{MediaSSRC: 123}
				if scenario == "FIR" {
					feedback = &rtcp.FullIntraRequest{FIR: []rtcp.FIREntry{{SSRC: 123, SequenceNumber: 1}}}
				}
				data, err := rtcp.Marshal([]rtcp.Packet{feedback})
				if err != nil {
					t.Fatal(err)
				}
				if err := b.HandleRTCP("viewer", data); err != nil {
					t.Fatal(err)
				}
				b.WriteSample(clockVideoSample(30*time.Millisecond), false)
			case "reconnect":
				b.SetPeerConnected("viewer", false)
				b.WriteSample(clockVideoSample(30*time.Millisecond), false)
				b.SetPeerConnected("viewer", true)
			case "overflow":
				b.WriteSample(clockVideoSample(30*time.Millisecond), false)
			}
			b.WriteSample(clockVideoSample(1900*time.Millisecond), false)
			b.WriteSample(clockVideoSample(40*time.Millisecond), true)
			writeClockSample(t, pt)

			if len(writer.timestamps) != 2 {
				t.Fatalf("sent %d RTP packets, want only the two IDRs", len(writer.timestamps))
			}
			// The initial 40 ms frame plus 2000 ms of discarded video at 90 kHz.
			if got := writer.timestamps[1] - writer.timestamps[0]; got != 183600 {
				t.Fatalf("recovery RTP timestamp delta = %d, want 183600", got)
			}
		})
	}
}

func TestOverflowIDRPreservesDiscardedRTPDuration(t *testing.T) {
	b, pt, writer := clockTestPeer(t)
	b.WriteSample(clockVideoSample(40*time.Millisecond), true)
	writeClockSample(t, pt)
	b.WriteSample(clockVideoSample(70*time.Millisecond), false)
	b.WriteSample(clockVideoSample(30*time.Millisecond), false)
	b.WriteSample(clockVideoSample(40*time.Millisecond), true)
	writeClockSample(t, pt)
	if len(writer.timestamps) != 2 || writer.timestamps[1]-writer.timestamps[0] != 12600 {
		t.Fatalf("overflow IDR compressed the RTP clock: %v", writer.timestamps)
	}
}

func TestRepeatedFeedbackPreservesAlreadyQueuedSkippedTime(t *testing.T) {
	b, pt, writer := clockTestPeer(t)
	b.WriteSample(clockVideoSample(40*time.Millisecond), true)
	writeClockSample(t, pt)
	data, err := rtcp.Marshal([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: 123}})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := b.HandleRTCP("viewer", data); err != nil {
			t.Fatal(err)
		}
		b.WriteSample(clockVideoSample(time.Second), false)
		b.WriteSample(clockVideoSample(40*time.Millisecond), true)
	}
	writeClockSample(t, pt)
	if len(writer.timestamps) != 2 || writer.timestamps[1]-writer.timestamps[0] != 187200 {
		t.Fatalf("repeated feedback lost queued skipped time: %v", writer.timestamps)
	}
}
