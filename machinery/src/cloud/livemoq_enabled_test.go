//go:build moq

package cloud

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kerberos-io/agent/machinery/src/cloud/livemoq"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/moq-dev/moq-go/moq"
)

func TestWatchLiveStreamMoQWritesClosesClientWhenContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	closed := make(chan struct{})
	var closeCalls atomic.Int32

	done := make(chan struct{})
	go func() {
		defer close(done)
		watchLiveStreamMoQWrites(ctx, func() error {
			if closeCalls.Add(1) == 1 {
				close(closed)
			}
			return nil
		}, &livemoq.WriteWatchdog{}, liveMoQConfig{quality: "low", sourceLabel: "sub"})
	}()

	cancel()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("watchLiveStreamMoQWrites() did not close the client after cancellation")
	}
	<-done
	if got := closeCalls.Load(); got != 1 {
		t.Fatalf("close calls = %d, want 1", got)
	}
}

func TestBoundedMoQDuration(t *testing.T) {
	const name = "AGENT_LIVE_MOQ_TEST_DURATION"
	tests := []struct {
		name string
		raw  string
		want time.Duration
	}{
		{name: "unset", want: 5 * time.Second},
		{name: "valid", raw: "12s", want: 12 * time.Second},
		{name: "invalid", raw: "later", want: 5 * time.Second},
		{name: "below minimum", raw: "100ms", want: time.Second},
		{name: "above maximum", raw: "2m", want: time.Minute},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(name, test.raw)
			if got := boundedMoQDuration(name, 5*time.Second, time.Second, time.Minute); got != test.want {
				t.Fatalf("boundedMoQDuration() = %s, want %s", got, test.want)
			}
		})
	}
}

func TestWatchLiveStreamMoQWritesRecordsTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	closed := make(chan struct{})
	communication := &models.Communication{}
	watchdog := &livemoq.WriteWatchdog{}
	watchdog.Begin(time.Now().Add(-time.Second))

	done := make(chan struct{})
	go func() {
		defer close(done)
		watchLiveStreamMoQWrites(ctx, func() error {
			close(closed)
			return nil
		}, watchdog, liveMoQConfig{
			quality:       models.StreamQualityLow,
			sourceLabel:   "sub",
			communication: communication,
			writeTimeout:  10 * time.Millisecond,
		})
	}()

	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("watchLiveStreamMoQWrites() did not close the client after a write timeout")
	}
	<-done
	if got := communication.RecoveryTelemetry().MoQWriteTimeouts; got != 1 {
		t.Fatalf("MoQ write timeouts = %d, want 1", got)
	}
}

func TestNativeMoQClientCloseStopsActiveWriter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server, err := moq.Listen(ctx, "127.0.0.1:0", moq.WithTLSGenerate("localhost"))
	if err != nil {
		t.Fatal(err)
	}
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- server.Serve(ctx)
	}()

	client, err := moq.Dial(ctx, "https://"+server.LocalAddr(), moq.WithTLSVerify(false))
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	broadcast, err := client.CreateBroadcast("test/native-close")
	if err != nil {
		client.Close()
		server.Close()
		t.Fatal(err)
	}
	stream, err := broadcast.PublishMedia("avc3", nil)
	if err != nil {
		client.Close()
		server.Close()
		t.Fatal(err)
	}

	firstWrite := make(chan struct{})
	writerDone := make(chan error, 1)
	go func() {
		payload := []byte{
			0x00, 0x00, 0x00, 0x01, 0x67, 0x42, 0xc0, 0x1e, 0xd9, 0x00, 0xa0, 0x47, 0xfe, 0xc8,
			0x00, 0x00, 0x00, 0x01, 0x68, 0xce, 0x38, 0x80,
			0x00, 0x00, 0x00, 0x01, 0x65, 0x88,
		}
		for timestamp := uint64(0); ; timestamp++ {
			err := stream.WriteFrame(moq.Frame{Payload: payload, TimestampUs: timestamp})
			if err != nil {
				writerDone <- err
				return
			}
			if timestamp == 0 {
				close(firstWrite)
			}
		}
	}()

	select {
	case <-firstWrite:
	case <-ctx.Done():
		t.Fatal("native MoQ writer did not start")
	}
	if err := stream.Finish(); err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-writerDone:
		if err == nil {
			t.Fatal("native MoQ writer stopped without an error after client close")
		}
	case <-ctx.Done():
		t.Fatal("native MoQ client close did not stop the writer")
	}

	_ = broadcast.Finish()
	_ = server.Close()
	select {
	case <-serveDone:
	case <-ctx.Done():
		t.Fatal("native MoQ server did not stop")
	}
}
