package capture

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

type keyframeCamera struct {
	stream   *gortsplib.ServerStream
	feedback chan rtcp.Packet
}

func (c *keyframeCamera) OnDescribe(*gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
	return &base.Response{StatusCode: base.StatusOK}, c.stream, nil
}

func (c *keyframeCamera) OnSetup(*gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
	return &base.Response{StatusCode: base.StatusOK}, c.stream, nil
}

func (c *keyframeCamera) OnPlay(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	ctx.Session.OnPacketRTCPAny(func(_ *description.Media, pkt rtcp.Packet) {
		c.feedback <- pkt
	})
	return &base.Response{StatusCode: base.StatusOK}, nil
}

func connectedKeyframeCamera(t *testing.T) (*Golibrtsp, *keyframeCamera) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	camera := &keyframeCamera{feedback: make(chan rtcp.Packet, 32)}
	server := &gortsplib.Server{
		RTSPAddress: "127.0.0.1:0",
		Handler:     camera,
		Listen: func(string, string) (net.Listener, error) {
			return listener, nil
		},
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Close)
	media := &description.Media{
		Type: description.MediaTypeVideo,
		Formats: []format.Format{&format.H264{
			PayloadTyp:        96,
			PacketizationMode: 1,
		}},
	}
	camera.stream = &gortsplib.ServerStream{
		Server: server,
		Desc:   &description.Session{Medias: []*description.Media{media}},
	}
	if err := camera.stream.Initialize(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(camera.stream.Close)

	client := &Golibrtsp{Url: "rtsp://" + listener.Addr().String() + "/camera"}
	t.Cleanup(func() {
		if err := client.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	if err := client.Connect(context.Background(), context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := client.RequestKeyframe(); err == nil {
		t.Fatal("requested a keyframe before the camera SSRC was known")
	}
	received := make(chan struct{}, 1)
	client.Client.OnPacketRTP(client.VideoH264Media, client.VideoH264Forma, func(*rtp.Packet) {
		received <- struct{}{}
	})
	if _, err := client.Client.Play(nil); err != nil {
		t.Fatal(err)
	}
	if err := camera.stream.WritePacketRTP(media, &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96,
			SSRC:           0x12345678,
			SequenceNumber: 1,
			Timestamp:      90000,
			Marker:         true,
		},
		Payload: []byte{0x65, 0x88, 0x84},
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-received:
	case <-time.After(3 * time.Second):
		t.Fatal("test camera RTP did not arrive")
	}
	return client, camera
}

func TestRTSPKeyframeRequestUsesCameraSessionSSRCs(t *testing.T) {
	client, camera := connectedKeyframeCamera(t)
	want := client.Client.Stats().Session.Medias[client.VideoH264Media].Formats[client.VideoH264Forma]
	if err := client.RequestKeyframe(); err != nil {
		t.Fatal(err)
	}
	select {
	case pkt := <-camera.feedback:
		pli, ok := pkt.(*rtcp.PictureLossIndication)
		if !ok {
			t.Fatalf("camera received %T, want PLI", pkt)
		}
		if pli.MediaSSRC != want.RemoteSSRC || pli.SenderSSRC != want.LocalSSRC {
			t.Fatalf("camera PLI SSRCs = sender %x media %x, want sender %x media %x",
				pli.SenderSSRC, pli.MediaSSRC, want.LocalSSRC, want.RemoteSSRC)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("camera did not receive RTCP PLI")
	}
}

func TestRTSPKeyframeRequestUnavailableBeforeConnectAndAfterClose(t *testing.T) {
	if err := (&Golibrtsp{}).RequestKeyframe(); err == nil {
		t.Fatal("uninitialized client accepted a keyframe request")
	}
	client, _ := connectedKeyframeCamera(t)
	if err := client.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := client.RequestKeyframe(); err == nil {
		t.Fatal("closed client accepted a keyframe request")
	}
}

func TestRTSPKeyframeRequestConcurrentWithClose(t *testing.T) {
	client, _ := connectedKeyframeCamera(t)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 5 {
				// Requests may race with Close and legitimately become unavailable.
				_ = client.RequestKeyframe()
			}
		}()
	}
	if err := client.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
}
