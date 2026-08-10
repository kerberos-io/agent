package components

import (
	"bufio"
	"context"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/kerberos-io/agent/machinery/src/capture"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/packets"
	"github.com/kerberos-io/joy4/av"
	"github.com/pion/rtp"
	"github.com/zaf/g711"
)

const (
	backchannelSampleRate       = 8000
	backchannelTalkspurtGap     = 500 * time.Millisecond
	backchannelReconnectInitial = time.Second
	backchannelReconnectMax     = 30 * time.Second
)

type backchannelClient interface {
	ConnectBackChannel(ctx context.Context, otelContext context.Context) error
	StartBackChannel(ctx context.Context, otelContext context.Context) error
	WritePacket(pkt packets.Packet) error
	Close(otelContext context.Context) error
}

type backchannelPacketizer struct {
	sequenceNumber uint16
	timestamp      uint32
	ssrc           uint32
	lastPacketAt   time.Time
}

func newBackchannelPacketizer() backchannelPacketizer {
	return backchannelPacketizer{
		sequenceNumber: uint16(rand.Uint32()),
		timestamp:      rand.Uint32(),
		ssrc:           rand.Uint32(),
	}
}

func (p *backchannelPacketizer) packet(audio models.AudioDataPartial, now time.Time) packets.Packet {
	bufferUlaw := make([]byte, len(audio.Data))
	for index, sample := range audio.Data {
		bufferUlaw[index] = g711.EncodeUlawFrame(sample)
	}

	pkt := packets.Packet{
		Packet: &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Marker:         p.lastPacketAt.IsZero() || now.Sub(p.lastPacketAt) >= backchannelTalkspurtGap,
				PayloadType:    0,
				SequenceNumber: p.sequenceNumber,
				Timestamp:      p.timestamp,
				SSRC:           p.ssrc,
			},
			Payload: bufferUlaw,
		},
	}

	p.timestamp += uint32(len(bufferUlaw))
	p.sequenceNumber++
	p.lastPacketAt = now

	return pkt
}

func GetBackChannelAudioCodec(streams []av.CodecData, communication *models.Communication) av.AudioCodecData {
	for _, stream := range streams {
		if stream.Type().IsAudio() {
			if stream.Type().String() == "PCM_MULAW" {
				pcmuCodec := stream.(av.AudioCodecData)
				if pcmuCodec.IsBackChannel() {
					communication.HasBackChannel = true
					return pcmuCodec
				}
			}
		}
	}
	return nil
}

func WriteAudioToBackchannel(communication *models.Communication, rtspClient capture.RTSPClient) {
	ctx := context.Background()
	if communication.Context != nil {
		ctx = *communication.Context
	}

	writeAudioToBackchannel(ctx, ctx, communication.HandleAudio, rtspClient)
}

func writeAudioToBackchannel(ctx context.Context, otelContext context.Context, audioChannel <-chan models.AudioDataPartial, rtspClient backchannelClient) {
	log.Log.Info("Audio.WriteAudioToBackchannel(): writing to backchannel audio codec")

	if err := rtspClient.StartBackChannel(ctx, otelContext); err != nil {
		log.Log.Error("Audio.WriteAudioToBackchannel(): error starting backchannel: " + err.Error())
		if !reconnectBackchannel(ctx, otelContext, rtspClient) {
			log.Log.Info("Audio.WriteAudioToBackchannel(): stopped while reconnecting")
			return
		}
	}

	packetizer := newBackchannelPacketizer()
	for {
		select {
		case <-ctx.Done():
			log.Log.Info("Audio.WriteAudioToBackchannel(): stopped")
			return
		case audio, ok := <-audioChannel:
			if !ok {
				log.Log.Info("Audio.WriteAudioToBackchannel(): finished")
				return
			}

			audio = latestBackchannelAudio(audio, audioChannel)
			if len(audio.Data) == 0 {
				continue
			}

			pkt := packetizer.packet(audio, time.Now())
			if err := rtspClient.WritePacket(pkt); err != nil {
				log.Log.Error("Audio.WriteAudioToBackchannel(): error writing packet to backchannel: " + err.Error())
				if !reconnectBackchannel(ctx, otelContext, rtspClient) {
					log.Log.Info("Audio.WriteAudioToBackchannel(): stopped while reconnecting")
					return
				}
				packetizer = newBackchannelPacketizer()
				continue
			}

			if !waitForBackchannel(ctx, time.Duration(len(audio.Data))*time.Second/backchannelSampleRate) {
				log.Log.Info("Audio.WriteAudioToBackchannel(): stopped")
				return
			}
		}
	}
}

func latestBackchannelAudio(audio models.AudioDataPartial, audioChannel <-chan models.AudioDataPartial) models.AudioDataPartial {
	for {
		select {
		case next, ok := <-audioChannel:
			if !ok {
				return audio
			}
			audio = next
		default:
			return audio
		}
	}
}

func reconnectBackchannel(ctx context.Context, otelContext context.Context, rtspClient backchannelClient) bool {
	backoff := backchannelReconnectInitial
	for {
		if err := rtspClient.Close(otelContext); err != nil {
			log.Log.Error("Audio.WriteAudioToBackchannel(): error closing failed backchannel: " + err.Error())
		}
		if ctx.Err() != nil {
			return false
		}

		err := rtspClient.ConnectBackChannel(ctx, otelContext)
		if err == nil {
			err = rtspClient.StartBackChannel(ctx, otelContext)
		}
		if err == nil {
			log.Log.Info("Audio.WriteAudioToBackchannel(): reconnected backchannel")
			return true
		}

		log.Log.Error("Audio.WriteAudioToBackchannel(): error reconnecting backchannel: " + err.Error())
		if !waitForBackchannel(ctx, backoff) {
			return false
		}
		backoff *= 2
		if backoff > backchannelReconnectMax {
			backoff = backchannelReconnectMax
		}
	}
}

func waitForBackchannel(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func WriteFileToBackChannel(infile av.DemuxCloser) {
	// Do the warmup!
	file, err := os.Open("./audiofile.bye")
	if err != nil {
		fmt.Println("WriteFileToBackChannel: error opening audiofile.bye file")
	}
	defer file.Close()

	// Read file into buffer
	reader := bufio.NewReader(file)
	buffer := make([]byte, 1024)

	count := 0
	for {
		_, err := reader.Read(buffer)
		if err != nil {
			break
		}
		// Send to backchannel
		infile.Write(buffer, 2, uint32(count))

		count = count + 1024
		time.Sleep(128 * time.Millisecond)
	}
}
