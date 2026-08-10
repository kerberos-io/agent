package components

import (
	"testing"
	"time"

	"github.com/kerberos-io/agent/machinery/src/models"
)

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