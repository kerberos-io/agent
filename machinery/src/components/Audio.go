package components

import (
	"bufio"
	"fmt"
	"os"
	"time"

	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/joy4/av"
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

func WriteAudioToBackchannel(infile av.DemuxCloser, streams []av.CodecData, communication *models.Communication) {
	log.Log.Info("WriteAudioToBackchannel: looking for backchannel audio codec")

	pcmuCodec := GetBackChannelAudioCodec(streams, communication)
	if pcmuCodec != nil {
		log.Log.Info("WriteAudioToBackchannel: found backchannel audio codec")

		length := 0
		channel := pcmuCodec.GetIndex() * 2 // This is the same calculation as Interleaved property in the SDP file.
		for audio := range communication.HandleAudio {
			// Encode PCM to MULAW
			var bufferUlaw []byte
			for _, v := range audio.Data {
				b := g711.EncodeUlawFrame(v)
				bufferUlaw = append(bufferUlaw, b)
			}
			infile.Write(bufferUlaw, channel, uint32(length))
			length = (length + len(bufferUlaw)) % 65536
			time.Sleep(128 * time.Millisecond)
		}
	}
	log.Log.Info("WriteAudioToBackchannel: finished")

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
