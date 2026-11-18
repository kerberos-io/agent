package webrtc

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kerberos-io/agent/machinery/src/capture"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/packets"
	"github.com/pion/rtcp"
	pionWebRTC "github.com/pion/webrtc/v4"
	pionMedia "github.com/pion/webrtc/v4/pkg/media"
)

func bufferedTestPeer(capacity int) *peerTrack {
	done := make(chan struct{})
	close(done)
	return &peerTrack{
		samples:       make(chan pionMedia.Sample, capacity),
		done:          done,
		needsKeyframe: true,
	}
}

func testVideoBroadcaster(t *testing.T) (*TrackBroadcaster, *peerTrack) {
	t.Helper()
	b := NewTrackBroadcaster(pionWebRTC.MimeTypeH264, "video", "test")
	pt := bufferedTestPeer(peerSampleBuffer)
	b.peers["viewer"] = pt
	t.Cleanup(b.Close)
	return b, pt
}

func TestVideoPeerStartsAndReconnectsAtKeyframe(t *testing.T) {
	b, pt := testVideoBroadcaster(t)
	sample := pionMedia.Sample{Data: []byte{1}}
	b.WriteSample(sample, true)
	if len(pt.samples) != 0 {
		t.Fatal("sent a keyframe before the transport connected")
	}

	b.SetPeerConnected("viewer", true)
	b.WriteSample(sample, false)
	if len(pt.samples) != 0 {
		t.Fatal("sent a dependent frame before the first IDR")
	}
	b.WriteSample(sample, true)
	b.WriteSample(sample, false)
	if len(pt.samples) != 2 || pt.needsKeyframe {
		t.Fatal("did not release video after the first IDR")
	}

	b.SetPeerConnected("viewer", false)
	b.WriteSample(sample, true)
	if len(pt.samples) != 0 {
		t.Fatal("retained or sent video while disconnected")
	}
	b.SetPeerConnected("viewer", true)
	b.WriteSample(sample, false)
	if len(pt.samples) != 0 {
		t.Fatal("reconnected viewer resumed on a dependent frame")
	}
	b.WriteSample(sample, true)
	if len(pt.samples) != 1 {
		t.Fatal("reconnected viewer did not resume at an IDR")
	}
}

func TestLateViewerDoesNotResetExistingViewer(t *testing.T) {
	b, first := testVideoBroadcaster(t)
	b.SetPeerConnected("viewer", true)
	b.WriteSample(pionMedia.Sample{}, true)
	<-first.samples

	late := bufferedTestPeer(peerSampleBuffer)
	b.peers["late"] = late
	b.SetPeerConnected("late", true)
	b.WriteSample(pionMedia.Sample{}, false)
	if len(first.samples) != 1 || len(late.samples) != 0 {
		t.Fatal("late viewer either interrupted an existing viewer or received a dependent frame")
	}
	b.WriteSample(pionMedia.Sample{}, true)
	if len(late.samples) != 1 {
		t.Fatal("late viewer did not receive the next IDR")
	}
}

func TestVideoFeedbackRequestsFreshKeyframe(t *testing.T) {
	for _, feedback := range []rtcp.Packet{
		&rtcp.PictureLossIndication{MediaSSRC: 123},
		&rtcp.FullIntraRequest{FIR: []rtcp.FIREntry{{SSRC: 123, SequenceNumber: 1}}},
	} {
		t.Run(fmt.Sprintf("%T", feedback), func(t *testing.T) {
			b, pt := testVideoBroadcaster(t)
			b.SetPeerConnected("viewer", true)
			b.WriteSample(pionMedia.Sample{}, true)
			b.WriteSample(pionMedia.Sample{}, false)

			rr, err := rtcp.Marshal([]rtcp.Packet{&rtcp.ReceiverReport{}})
			if err != nil {
				t.Fatal(err)
			}
			if err := b.HandleRTCP("viewer", rr); err != nil || len(pt.samples) != 2 {
				t.Fatal("receiver report reset the video gate", err)
			}
			data, err := rtcp.Marshal([]rtcp.Packet{&rtcp.ReceiverReport{}, feedback})
			if err != nil {
				t.Fatal(err)
			}
			if err := b.HandleRTCP("viewer", data); err != nil {
				t.Fatal(err)
			}
			b.WriteSample(pionMedia.Sample{}, false)
			if len(pt.samples) != 0 || !pt.needsKeyframe {
				t.Fatal("PLI/FIR did not discard stale queued frames and wait for an IDR")
			}
			b.WriteSample(pionMedia.Sample{}, true)
			if len(pt.samples) != 1 {
				t.Fatal("PLI/FIR recovery did not resume at an IDR")
			}
			if err := b.HandleRTCP("viewer", []byte{0}); err == nil {
				t.Fatal("malformed RTCP was accepted")
			}
		})
	}
}

func TestSlowVideoPeerRecoversWithoutBlockingOthers(t *testing.T) {
	for _, overflowIsKeyframe := range []bool{false, true} {
		t.Run(map[bool]string{false: "dependent-frame-dropped", true: "new-IDR-replaces-backlog"}[overflowIsKeyframe], func(t *testing.T) {
			b, fast := testVideoBroadcaster(t)
			slow := bufferedTestPeer(2)
			b.peers["slow"] = slow
			b.SetPeerConnected("viewer", true)
			b.SetPeerConnected("slow", true)
			b.WriteSample(pionMedia.Sample{}, true)
			b.WriteSample(pionMedia.Sample{}, false)
			b.WriteSample(pionMedia.Sample{}, overflowIsKeyframe)
			if len(fast.samples) != 3 {
				t.Fatal("slow viewer interrupted another viewer")
			}
			if overflowIsKeyframe {
				if len(slow.samples) != 1 || slow.needsKeyframe {
					t.Fatal("fresh IDR did not replace the stale backlog")
				}
			} else {
				b.WriteSample(pionMedia.Sample{}, false)
				if len(slow.samples) != 0 || !slow.needsKeyframe {
					t.Fatal("slow viewer forwarded dependent frames after a drop")
				}
				b.WriteSample(pionMedia.Sample{}, true)
				if len(slow.samples) != 1 {
					t.Fatal("slow viewer did not recover at the next IDR")
				}
			}
		})
	}
}

func TestAudioDoesNotWaitForVideoKeyframe(t *testing.T) {
	b := NewTrackBroadcaster(pionWebRTC.MimeTypePCMU, "audio", "test")
	pt := bufferedTestPeer(2)
	pt.connected = true
	pt.needsKeyframe = false
	b.peers["viewer"] = pt
	t.Cleanup(b.Close)

	b.SetPeerConnected("viewer", false)
	for range 3 {
		b.WriteSample(pionMedia.Sample{}, false)
	}
	if len(pt.samples) != 2 {
		t.Fatal("audio queue behavior changed")
	}
}

func TestVideoSampleBufferPreservesKeyframeFlag(t *testing.T) {
	b, pt := testVideoBroadcaster(t)
	b.SetPeerConnected("viewer", true)
	state := &streamState{}
	first := packets.Packet{Data: []byte{0x65}, IsKeyFrame: true, Time: 1}
	processVideoPacket(first, state, b, models.Config{})
	processVideoPacket(packets.Packet{Data: []byte{0x41}, Time: 34}, state, b, models.Config{})
	if len(pt.samples) != 1 || !bytes.Equal((<-pt.samples).Data, first.Data) {
		t.Fatal("buffered IDR lost its keyframe flag")
	}

	b.SetPeerConnected("viewer", false)
	b.SetPeerConnected("viewer", true)
	writeFinalSamples(state, b, nil)
	if len(pt.samples) != 0 {
		t.Fatal("final dependent frame was mislabeled as an IDR")
	}
	state.lastVideoSample = &pionMedia.Sample{Data: first.Data}
	state.lastVideoKeyframe = true
	writeFinalSamples(state, b, nil)
	if len(pt.samples) != 1 {
		t.Fatal("final IDR lost its keyframe flag")
	}
}

func TestKeyframeRequestsCoalesceAndRetryUntilIDR(t *testing.T) {
	b, _ := testVideoBroadcaster(t)
	calls := make(chan time.Time, 10)
	var count atomic.Int32
	b.requestKeyframe = func() error {
		calls <- time.Now()
		if count.Add(1) == 1 {
			return errors.New("camera is not ready yet")
		}
		b.WriteSample(pionMedia.Sample{}, true)
		return nil
	}
	for _, id := range []string{"viewer", "second", "third"} {
		if id != "viewer" {
			b.mu.Lock()
			b.peers[id] = bufferedTestPeer(peerSampleBuffer)
			b.mu.Unlock()
		}
		b.SetPeerConnected(id, true)
	}
	var first time.Time
	select {
	case first = <-calls:
	case <-time.After(2 * time.Second):
		t.Fatal("connected viewer did not request a keyframe")
	}
	feedback, err := rtcp.Marshal([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: 123}})
	if err != nil {
		t.Fatal(err)
	}
	for range 100 {
		if err := b.HandleRTCP("viewer", feedback); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case second := <-calls:
		if second.Sub(first) < keyframeRequestInterval-5*time.Millisecond {
			t.Fatal("feedback bypassed the per-stream request rate limit")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("failed camera request was not retried")
	}
	select {
	case <-calls:
		t.Fatal("requests continued after all viewers received an IDR")
	case <-time.After(keyframeRequestInterval + 100*time.Millisecond):
	}
}

func TestKeyframeRequestsStopOnDisconnectAndClose(t *testing.T) {
	b, _ := testVideoBroadcaster(t)
	var calls atomic.Int32
	b.requestKeyframe = func() error { calls.Add(1); return nil }
	b.lastRequest = time.Now()
	b.SetPeerConnected("viewer", true)
	b.SetPeerConnected("viewer", false)
	time.Sleep(keyframeRequestInterval + 50*time.Millisecond)
	if calls.Load() != 0 {
		t.Fatal("disconnected viewer requested a keyframe")
	}
	b.SetPeerConnected("viewer", true)
	b.Close()
	if _, err := b.AddPeer("closed"); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("AddPeer() after Close() = %v, want closed pipe", err)
	}
}

func TestBroadcasterCloseWaitsForKeyframeRequest(t *testing.T) {
	b, _ := testVideoBroadcaster(t)
	started, release, closed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	b.requestKeyframe = func() error {
		close(started)
		<-release
		return nil
	}
	b.SetPeerConnected("viewer", true)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("request did not start")
	}
	go func() { b.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("Close returned before the in-flight camera request completed")
	case <-time.After(30 * time.Millisecond):
	}
	unblock()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not finish")
	}
}

func TestConcurrentVideoFeedbackAndPeerLifecycle(t *testing.T) {
	b := NewTrackBroadcaster(pionWebRTC.MimeTypeH264, "video", "test")
	t.Cleanup(b.Close)
	data, err := rtcp.Marshal([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: 123}})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, id := range []string{"first", "second", "third"} {
		if _, err := b.AddPeer(id); err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range 200 {
				b.SetPeerConnected(id, j%5 != 0)
				b.WriteSample(pionMedia.Sample{Data: []byte{0, 0, 0, 1, 0x65}, Duration: time.Millisecond}, j%10 == 0)
				if err := b.HandleRTCP(id, data); err != nil {
					t.Error(err)
				}
			}
			b.RemovePeer(id)
		}()
	}
	wg.Wait()
}

type keyframeTestSource struct {
	capture.RTSPClient
	request func() error
}

func (s keyframeTestSource) RequestKeyframe() error { return s.request() }

func TestVideoBroadcastersRequestTheirOwnSource(t *testing.T) {
	streams := []packets.Stream{{Name: "H264", IsVideo: true}}
	var mainCalls, subCalls atomic.Int32
	main := NewVideoBroadcaster(streams, keyframeTestSource{request: func() error { mainCalls.Add(1); return nil }})
	sub := NewVideoBroadcaster(streams, keyframeTestSource{request: func() error { subCalls.Add(1); return nil }})
	t.Cleanup(main.Close)
	t.Cleanup(sub.Close)
	if err := main.requestKeyframe(); err != nil {
		t.Fatal(err)
	}
	if mainCalls.Load() != 1 || subCalls.Load() != 0 {
		t.Fatal("main feedback reached the wrong source")
	}
	if err := sub.requestKeyframe(); err != nil {
		t.Fatal(err)
	}
	if mainCalls.Load() != 1 || subCalls.Load() != 1 {
		t.Fatal("sub feedback reached the wrong source")
	}
}
