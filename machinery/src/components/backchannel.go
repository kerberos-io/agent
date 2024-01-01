package components

import (
	"bufio"
	"fmt"
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
	log.Log.Info("Audio.WriteAudioToBackchannel(): writing to backchannel audio codec")
	length := uint32(0)
	sequenceNumber := uint16(0)
	for audio := range communication.HandleAudio {
		// Encode PCM to MULAW
		var bufferUlaw []byte
		for _, v := range audio.Data {
			b := g711.EncodeUlawFrame(v)
			bufferUlaw = append(bufferUlaw, b)
		}

		pkt := packets.Packet{
			Packet: &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         true, // should be true
					PayloadType:    0,    //packet.PayloadType, // will be owerwriten
					SequenceNumber: sequenceNumber,
					Timestamp:      uint32(length),
					SSRC:           1293847657,
				},
				Payload: bufferUlaw,
			},
		}
		err := rtspClient.WritePacket(pkt)
		if err != nil {
			log.Log.Error("Audio.WriteAudioToBackchannel(): error writing packet to backchannel")
		}

		length = (length + uint32(len(bufferUlaw))) % 65536
		sequenceNumber = (sequenceNumber + 1) % 65535
		time.Sleep(128 * time.Millisecond)
	}
	log.Log.Info("Audio.WriteAudioToBackchannel(): finished")

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
		fmt.Println(buffer)
		infile.Write(buffer, 2, uint32(count))

		count = count + 1024
		time.Sleep(128 * time.Millisecond)
	}
}
