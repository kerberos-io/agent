package components

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/kerberos-io/agent/machinery/src/capture"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/packets"
	"github.com/kerberos-io/agent/machinery/src/utils"
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
		fmt.Println("WriteFileToBackChannel: bufferUlaw", bufferUlaw)

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
	}
	log.Log.Info("Audio.WriteAudioToBackchannel(): finished")

}

func WriteFileToBackChannel(communication *models.Communication, rtspClient capture.RTSPClient) {
	log.Log.Info("Audio.WriteFileToBackChannel(): writing to backchannel audio codec")
	length := uint32(0)
	sequenceNumber := uint16(0)

	// Do the warmup!
	file, err := os.Open("./audio/police-siren.wav")
	if err != nil {
		fmt.Println("WriteFileToBackChannel: error opening audiofile.bye file")
	}
	defer file.Close()

	time.Sleep(3 * time.Second)

	// Create a random sequence number
	ssrc := utils.RandomUint32()

	// Read file into buffer
	reader := bufio.NewReader(file)
	buffer := make([]byte, 1024)
	for {
		_, err := reader.Read(buffer)
		if err != nil {
			break
		}
		// Encode PCM to MULAW
		bufferUlaw := g711.EncodeUlaw(buffer)
		fmt.Println("WriteFileToBackChannel: bufferUlaw", bufferUlaw)

		pkt := packets.Packet{
			Packet: &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					Marker:         false, // should be true
					PayloadType:    0,     //packet.PayloadType, // will be owerwriten
					SequenceNumber: sequenceNumber,
					Timestamp:      uint32(length),
					SSRC:           ssrc,
				},
				Payload: bufferUlaw,
			},
		}
		err = rtspClient.WritePacket(pkt)
		if err != nil {
			log.Log.Error("Audio.WriteFileToBackChannel(): error writing packet to backchannel")
			if err.Error() == "EOF" {
				log.Log.Info("Audio.WriteFileToBackChannel(): EOF, restarting backchannel")
				rtspClient.Close()
				err = rtspClient.ConnectBackChannel(context.Background())
				if err != nil {
					log.Log.Error("Audio.WriteFileToBackChannel(): error connecting to backchannel")
					fmt.Println(err)
				}
				err = rtspClient.StartBackChannel(context.Background())
				if err != nil {
					log.Log.Error("Audio.WriteFileToBackChannel(): error starting backchannel")
					fmt.Println(err)
				}
			}
		} else {
			log.Log.Info("Audio.WriteFileToBackChannel(): wrote packet to backchannel")
		}

		length = (length + uint32(len(buffer))) % 65536
		sequenceNumber = (sequenceNumber + 1) % 65535
	}
	log.Log.Info("Audio.WriteAudioToBaWriteFileToBackChannelckchannel(): finished")

}
