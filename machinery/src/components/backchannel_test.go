package components

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/packets"
)

type fakeBackchannelClient struct {
	mutex           sync.Mutex
	startErrors     []error
	connectError    error
	writeErrors     []error
	startCalls      int
	connectCalls    int
	closeCalls      int
	writeCalls      int
	connectAttempt  chan struct{}
	successfulWrite chan packets.Packet
}

func (f *fakeBackchannelClient) ConnectBackChannel(context.Context, context.Context) error {
	f.mutex.Lock()
	f.connectCalls++
	err := f.connectError
	f.mutex.Unlock()

	select {
	case f.connectAttempt <- struct{}{}:
	default:
	}
	return err
}

func (f *fakeBackchannelClient) StartBackChannel(context.Context, context.Context) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.startCalls++
	if len(f.startErrors) == 0 {
		return nil
	}
	err := f.startErrors[0]
	f.startErrors = f.startErrors[1:]
	return err
}

func (f *fakeBackchannelClient) WritePacket(pkt packets.Packet) error {
	f.mutex.Lock()
	f.writeCalls++
	var err error
	if len(f.writeErrors) != 0 {
		err = f.writeErrors[0]
		f.writeErrors = f.writeErrors[1:]
	}
	f.mutex.Unlock()

	if err == nil {
		select {
		case f.successfulWrite <- pkt:
		default:
		}
	}
	return err
}

func (f *fakeBackchannelClient) Close(context.Context) error {
	f.mutex.Lock()
	f.closeCalls++
	f.mutex.Unlock()
	return nil
}

func (f *fakeBackchannelClient) callCounts() (start, connect, close, write int) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return f.startCalls, f.connectCalls, f.closeCalls, f.writeCalls
}

func TestBackchannelPacketizerUsesFullRTPClock(t *testing.T) {
	packetizer := backchannelPacketizer{ssrc: 1}
	audio := models.AudioDataPartial{Data: make([]int16, 1024)}
	startedAt := time.Unix(1, 0)

	var timestamp uint32
	for index := 0; index <= 64; index++ {
		pkt := packetizer.packet(audio, startedAt.Add(time.Duration(index)*128*time.Millisecond))
		timestamp = pkt.Packet.Timestamp
	}

	if timestamp != 65536 {
		t.Fatalf("timestamp after 64 frames = %d, want 65536", timestamp)
	}
}

func TestBackchannelPacketizerUsesNaturalSequenceRollover(t *testing.T) {
	packetizer := backchannelPacketizer{sequenceNumber: ^uint16(0), ssrc: 1}
	audio := models.AudioDataPartial{Data: []int16{0}}
	startedAt := time.Unix(1, 0)

	last := packetizer.packet(audio, startedAt)
	firstAfterRollover := packetizer.packet(audio, startedAt.Add(time.Millisecond))

	if last.Packet.SequenceNumber != ^uint16(0) {
		t.Fatalf("last sequence number = %d, want %d", last.Packet.SequenceNumber, ^uint16(0))
	}
	if firstAfterRollover.Packet.SequenceNumber != 0 {
		t.Fatalf("first sequence number after rollover = %d, want 0", firstAfterRollover.Packet.SequenceNumber)
	}
}

func TestBackchannelPacketizerMarksTalkspurtStart(t *testing.T) {
	packetizer := backchannelPacketizer{ssrc: 1}
	audio := models.AudioDataPartial{Data: []int16{0}}
	startedAt := time.Unix(1, 0)

	first := packetizer.packet(audio, startedAt)
	continuous := packetizer.packet(audio, startedAt.Add(128*time.Millisecond))
	afterGap := packetizer.packet(audio, startedAt.Add(backchannelTalkspurtGap+128*time.Millisecond))

	if !first.Packet.Marker {
		t.Fatal("first packet must mark the start of a talkspurt")
	}
	if continuous.Packet.Marker {
		t.Fatal("continuous packet must not carry the marker bit")
	}
	if !afterGap.Packet.Marker {
		t.Fatal("packet after an audio gap must mark a new talkspurt")
	}
}

func TestWriteAudioToBackchannelReconnectsAfterWriteFailure(t *testing.T) {
	writeFailure := errors.New("EOF")
	client := &fakeBackchannelClient{
		writeErrors:     []error{writeFailure, nil},
		connectAttempt:  make(chan struct{}, 1),
		successfulWrite: make(chan packets.Packet, 1),
	}
	audioChannel := make(chan models.AudioDataPartial, 2)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		writeAudioToBackchannel(ctx, ctx, audioChannel, client)
		close(done)
	}()

	audioChannel <- models.AudioDataPartial{Data: make([]int16, 1024)}
	select {
	case <-client.connectAttempt:
	case <-time.After(time.Second):
		t.Fatal("backchannel was not reconnected after the write failure")
	}

	audioChannel <- models.AudioDataPartial{Data: make([]int16, 1024)}
	select {
	case pkt := <-client.successfulWrite:
		if !pkt.Packet.Marker {
			t.Fatal("first packet after reconnect must mark a new talkspurt")
		}
	case <-time.After(time.Second):
		t.Fatal("fresh audio was not written after reconnect")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("backchannel writer did not stop after cancellation")
	}

	startCalls, connectCalls, closeCalls, writeCalls := client.callCounts()
	if startCalls != 2 || connectCalls != 1 || closeCalls != 1 || writeCalls != 2 {
		t.Fatalf("calls (start, connect, close, write) = (%d, %d, %d, %d), want (2, 1, 1, 2)", startCalls, connectCalls, closeCalls, writeCalls)
	}
}

func TestWriteAudioToBackchannelCancellationStopsReconnect(t *testing.T) {
	client := &fakeBackchannelClient{
		connectError:    errors.New("camera unavailable"),
		writeErrors:     []error{errors.New("EOF")},
		connectAttempt:  make(chan struct{}, 1),
		successfulWrite: make(chan packets.Packet, 1),
	}
	audioChannel := make(chan models.AudioDataPartial, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		writeAudioToBackchannel(ctx, ctx, audioChannel, client)
		close(done)
	}()

	audioChannel <- models.AudioDataPartial{Data: make([]int16, 1024)}
	select {
	case <-client.connectAttempt:
	case <-time.After(time.Second):
		t.Fatal("expected a reconnect attempt")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("cancellation did not interrupt reconnect backoff")
	}
}
