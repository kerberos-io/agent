package webrtc

import (
	"encoding/base64"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/packets"
	"github.com/pion/rtcp"
	pionWebRTC "github.com/pion/webrtc/v4"
	pionMedia "github.com/pion/webrtc/v4/pkg/media"
)

func TestBuildICEServersOmitsEmptyURLs(t *testing.T) {
	webRTC := CreateWebRTC("camera", []string{""}, []string{""}, "", "")

	iceServers := buildICEServers(*webRTC)

	if len(iceServers) != 0 {
		t.Fatalf("buildICEServers() returned %d servers, want 0", len(iceServers))
	}
}

func TestBuildICEServersIncludesConfiguredURLs(t *testing.T) {
	webRTC := CreateWebRTC(
		"camera",
		[]string{"", " stun:turn-fra1.kerberos.io:3478 "},
		[]string{" turn:turn-fra1.kerberos.io:3478 "},
		"username",
		"credential",
	)

	iceServers := buildICEServers(*webRTC)

	if len(iceServers) != 2 {
		t.Fatalf("buildICEServers() returned %d servers, want 2", len(iceServers))
	}

	if got := iceServers[0].URLs[0]; got != "stun:turn-fra1.kerberos.io:3478" {
		t.Fatalf("STUN URL = %q, want trimmed URL", got)
	}
	if got := iceServers[1].URLs[0]; got != "turn:turn-fra1.kerberos.io:3478" {
		t.Fatalf("TURN URL = %q, want trimmed URL", got)
	}
	if iceServers[1].Username != "username" || iceServers[1].Credential != "credential" {
		t.Fatal("TURN credentials were not preserved")
	}
}

func TestWebRTCConnectionRequestsStartupAndRecoveryKeyframes(t *testing.T) {
	viewer, err := pionWebRTC.NewPeerConnection(pionWebRTC.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := viewer.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := viewer.AddTransceiverFromKind(pionWebRTC.RTPCodecTypeVideo, pionWebRTC.RTPTransceiverInit{
		Direction: pionWebRTC.RTPTransceiverDirectionRecvonly,
	}); err != nil {
		t.Fatal(err)
	}
	tracks := make(chan *pionWebRTC.TrackRemote, 1)
	viewer.OnTrack(func(track *pionWebRTC.TrackRemote, _ *pionWebRTC.RTPReceiver) {
		tracks <- track
	})
	offer, err := viewer.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	gathered := pionWebRTC.GatheringCompletePromise(viewer)
	if err := viewer.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gathered:
	case <-time.After(5 * time.Second):
		t.Fatal("viewer ICE gathering timed out")
	}

	requests := make(chan time.Time, 10)
	b := NewVideoBroadcaster([]packets.Stream{{Name: "H264", IsVideo: true}}, keyframeTestSource{
		request: func() error { requests <- time.Now(); return nil },
	})
	t.Cleanup(b.Close)
	const sessionKey = "unit-camera/startup-recovery"
	t.Cleanup(func() {
		globalConnectionManager.mu.RLock()
		wrapper := globalConnectionManager.peerConnections[sessionKey]
		globalConnectionManager.mu.RUnlock()
		if wrapper != nil {
			cleanupPeerConnection(sessionKey, wrapper)
		}
	})
	var signalingMu sync.Mutex
	var pending []pionWebRTC.ICECandidateInit
	signaling := &models.LiveHDSignalingCallbacks{
		SendCandidate: func(_ string, data string) error {
			var candidate pionWebRTC.ICECandidateInit
			if err := json.Unmarshal([]byte(data), &candidate); err != nil {
				return err
			}
			signalingMu.Lock()
			defer signalingMu.Unlock()
			if viewer.RemoteDescription() == nil {
				pending = append(pending, candidate)
				return nil
			}
			return viewer.AddICECandidate(candidate)
		},
		SendAnswer: func(_ string, data string) error {
			answer, err := base64.StdEncoding.DecodeString(data)
			if err != nil {
				return err
			}
			signalingMu.Lock()
			defer signalingMu.Unlock()
			if err := viewer.SetRemoteDescription(pionWebRTC.SessionDescription{
				Type: pionWebRTC.SDPTypeAnswer, SDP: string(answer),
			}); err != nil {
				return err
			}
			for _, candidate := range pending {
				if err := viewer.AddICECandidate(candidate); err != nil {
					return err
				}
			}
			pending = nil
			return nil
		},
	}
	InitializeWebRTCConnection(
		&models.Configuration{Config: models.Config{Key: "unit-camera"}},
		&models.Communication{}, nil, b, nil,
		models.LiveHDHandshake{
			Signaling: signaling,
			Payload: models.RequestHDStreamPayload{
				SessionID:          "startup-recovery",
				SessionDescription: base64.StdEncoding.EncodeToString([]byte(viewer.LocalDescription().SDP)),
			},
		},
	)
	var firstRequest time.Time
	select {
	case firstRequest = <-requests:
	case <-time.After(5 * time.Second):
		t.Fatal("connected transport did not request a startup keyframe")
	}
	b.WriteSample(pionMedia.Sample{Data: []byte{0, 0, 0, 1, 0x41, 0x80}, Duration: time.Second / 30}, false)
	select {
	case <-tracks:
		t.Fatal("viewer received dependent video before its startup IDR")
	case <-time.After(100 * time.Millisecond):
	}
	b.WriteSample(pionMedia.Sample{Data: []byte{0, 0, 0, 1, 0x65, 0x88, 0x84}, Duration: time.Second / 30}, true)
	var track *pionWebRTC.TrackRemote
	select {
	case track = <-tracks:
	case <-time.After(3 * time.Second):
		t.Fatal("viewer did not receive the requested startup IDR")
	}

	for _, feedback := range []rtcp.Packet{
		&rtcp.PictureLossIndication{SenderSSRC: 42, MediaSSRC: uint32(track.SSRC())},
		&rtcp.FullIntraRequest{SenderSSRC: 42, FIR: []rtcp.FIREntry{{SSRC: uint32(track.SSRC()), SequenceNumber: 1}}},
	} {
		if err := viewer.WriteRTCP([]rtcp.Packet{feedback}); err != nil {
			t.Fatal(err)
		}
		select {
		case at := <-requests:
			if at.Sub(firstRequest) < keyframeRequestInterval-5*time.Millisecond {
				t.Fatal("real viewer feedback bypassed keyframe request throttling")
			}
			firstRequest = at
		case <-time.After(3 * time.Second):
			t.Fatalf("real %T feedback did not request a recovery keyframe", feedback)
		}
		b.WriteSample(pionMedia.Sample{Data: []byte{0, 0, 0, 1, 0x65, 0x88, 0x84}, Duration: time.Second / 30}, true)
	}
}
