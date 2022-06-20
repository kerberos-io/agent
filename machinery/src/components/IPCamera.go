package components

import (
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/joy4/av/pubsub"
	"github.com/kerberos-io/joy4/format/mp4"

	"github.com/kerberos-io/joy4/av"
	"github.com/kerberos-io/joy4/av/avutil"
	"github.com/kerberos-io/joy4/cgo/ffmpeg"
	"github.com/kerberos-io/joy4/format"
)

func OpenRTSP(url string) (av.DemuxCloser, []av.CodecData, error) {
	format.RegisterAll()
	infile, err := avutil.Open(url)
	if err == nil {
		streams, errstreams := infile.Streams()
		return infile, streams, errstreams
	}
	return nil, []av.CodecData{}, err
}

func GetVideoDecoder(streams []av.CodecData) *ffmpeg.VideoDecoder {
	// Load video codec
	var vstream av.VideoCodecData
	for _, stream := range streams {
		if stream.Type().IsAudio() {
			//astream := stream.(av.AudioCodecData)
		} else if stream.Type().IsVideo() {
			vstream = stream.(av.VideoCodecData)
		}
	}
	dec, _ := ffmpeg.NewVideoDecoder(vstream)
	return dec
}

func HandleStream(infile av.DemuxCloser, queue *pubsub.Queue, communication *models.Communication, wg *sync.WaitGroup) {

	log.Log.Debug("HandleStream: started")
	var err error
loop:
	for {

		// This will check if we need to stop the thread,
		// because of a reconfiguration.
		select {
		case <-communication.HandleStream:
			break loop
		default:
		}

		var pkt av.Packet
		if pkt, err = infile.ReadPacket(); err != nil { // sometimes this throws an end of file..
			log.Log.Info(strconv.Itoa(len(pkt.Data)))
			log.Log.Error(err.Error())
			if err.Error() == "EOF" {
				time.Sleep(30 * time.Second)
			}
		}

		// Could be that a decode is throwing errors.
		if len(pkt.Data) > 0 {

			queue.WritePacket(pkt)

			// This will check if we need to stop the thread,
			// because of a reconfiguration.
			select {
			case <-communication.HandleStream:
				break loop
			default:
			}

			/*select {
			case packetsBuffer <- pkt:
			default:
			}

			select {
			case webrtcPacketsRealtimeStream <- pkt:
			default:
			}*/

			if pkt.IsKeyFrame {

				// Increment packets, so we know the device
				// is not blocking.
				r := communication.PackageCounter.Load().(int64)
				log.Log.Info("HandleStream: packet size " + strconv.Itoa(len(pkt.Data)))
				communication.PackageCounter.Store((r + 1) % 1000)

				/*select {
				case packetsRealtime <- pkt:
				default:
				}
				select {
				case packetsRealtimeStream <- pkt:
				default:
				}*/
			}
		}
	}
	wg.Done()
	log.Log.Debug("HandleStream: finished")
}

func RecordStream(recordingCursor *pubsub.QueueCursor, motion chan int64, devicename string, config *models.Config, streams []av.CodecData) {
	log.Log.Debug("RecordStream: started")

	recordingPeriod := config.Capture.PostRecording         // number of seconds to record.
	maxRecordingPeriod := config.Capture.MaxLengthRecording // maximum number of seconds to record.

	// Synchronise the last synced time
	now := time.Now().Unix()
	startRecording := now
	timestamp := now

	// Check if continuous recording.
	if config.Capture.Continuous == "true" {

		// Do not do anything!
		log.Log.Info("Start continuous recording ")

		loc, _ := time.LoadLocation(config.Timezone)
		now = time.Now().Unix()
		timestamp = now
		start := false
		var name string
		var myMuxer *mp4.Muxer
		var file *os.File
		var err error

		// If continuous record the full length
		recordingPeriod = maxRecordingPeriod
		// Recording file name
		fullName := ""

		// Get as much packets we need.
		//for pkt := range packets {
		var cursorError error
		var pkt av.Packet

		for cursorError == nil {

			pkt, cursorError = recordingCursor.ReadPacket()

			now := time.Now().Unix()

			if start && // If already recording and current frame is a keyframe and we should stop recording
				pkt.IsKeyFrame && (timestamp+recordingPeriod-now <= 0 || now-startRecording >= maxRecordingPeriod) {

				// This will write the trailer a well.
				if err := myMuxer.WriteTrailer(); err != nil {
					log.Log.Error(err.Error())
				}

				log.Log.Info("Recording finished: file save: " + name)
				file.Close()

				// Check if need to convert to fragmented using bento
				if config.Capture.Fragmented == "true" && config.Capture.FragmentedDuration > 0 {
					CreateFragmentedMP4(fullName, config.Capture.FragmentedDuration)
				}

				// Create a symbol link.
				fc, _ := os.Create("./data/cloud/" + name)
				fc.Close()

				// Cleanup muxer
				start = false
				myMuxer = nil
				runtime.GC()
				debug.FreeOSMemory()
			}

			// If not yet started and a keyframe, let's make a recording
			if !start && pkt.IsKeyFrame {

				// Check if within time interval
				nowInTimezone := time.Now().In(loc)
				weekday := nowInTimezone.Weekday()
				hour := nowInTimezone.Hour()
				minute := nowInTimezone.Minute()
				second := nowInTimezone.Second()
				timeEnabled := config.Time
				timeInterval := config.Timetable[int(weekday)]

				if timeEnabled == "true" && timeInterval != nil {
					start1 := timeInterval.Start1
					end1 := timeInterval.End1
					start2 := timeInterval.Start2
					end2 := timeInterval.End2
					currentTimeInSeconds := hour*60*60 + minute*60 + second
					if (currentTimeInSeconds >= start1 && currentTimeInSeconds <= end1) ||
						(currentTimeInSeconds >= start2 && currentTimeInSeconds <= end2) {

					} else {
						log.Log.Debug("Disabled: no continuous recording at this moment. Not within specified time interval.")
						time.Sleep(5 * time.Second)
						continue
					}
				}

				start = true
				timestamp = now

				// timestamp_microseconds_instanceName_regionCoordinates_numberOfChanges_token
				// 1564859471_6-474162_oprit_577-283-727-375_1153_27.mp4
				// - Timestamp
				// - Size + - + microseconds
				// - device
				// - Region
				// - Number of changes
				// - Token

				startRecording = time.Now().Unix() // we mark the current time when the record started.ss
				s := strconv.FormatInt(startRecording, 10) + "_" + "6" + "-" + "967003" + "_" + config.Name + "_" + "200-200-400-400" + "_" + "24" + "_" + "769"
				name = s + ".mp4"
				fullName = "./data/recordings/" + name

				// Running...
				log.Log.Info("Recording started")

				file, err = os.Create(fullName)
				if err == nil {
					myMuxer = mp4.NewMuxer(file)
				}

				log.Log.Info("Recording starting: composing recording")
				log.Log.Info("Recording starting: write header")

				// Creating the file, might block sometimes.
				if err := myMuxer.WriteHeader(streams); err != nil {
					log.Log.Error(err.Error())
				}

				if err := myMuxer.WritePacket(pkt); err != nil {
					log.Log.Error(err.Error())
				}

			} else if start {
				if err := myMuxer.WritePacket(pkt); err != nil {
					log.Log.Error(err.Error())
				}
			}
		}

	} else {

		log.Log.Info("Start motion based recording ")

		var myMuxer *mp4.Muxer
		var file *os.File
		var err error

		for _ = range motion {

			now = time.Now().Unix()
			timestamp = now

			// timestamp_microseconds_instanceName_regionCoordinates_numberOfChanges_token
			// 1564859471_6-474162_oprit_577-283-727-375_1153_27.mp4
			// - Timestamp
			// - Size + - + microseconds
			// - device
			// - Region
			// - Number of changes
			// - Token

			startRecording = time.Now().Unix() // we mark the current time when the record started.ss
			s := strconv.FormatInt(startRecording, 10) + "_" + "6" + "-" + "967003" + "_" + config.Name + "_" + "200-200-400-400" + "_" + "24" + "_" + "769"
			name := s + ".mp4"
			fullName := "./data/recordings/" + name

			// Running...
			log.Log.Info("Recording started")
			file, err = os.Create(fullName)
			if err == nil {
				myMuxer = mp4.NewMuxer(file)
			}

			start := false

			log.Log.Info("Recording starting: composing recording")
			log.Log.Info("Recording starting: write header")
			// Creating the file, might block sometimes.
			if err := myMuxer.WriteHeader(streams); err != nil {
				log.Log.Error(err.Error())
			}

			// Get as much packets we need.
			//for pkt := range packets {

			var cursorError error
			var pkt av.Packet

			for cursorError == nil {

				pkt, cursorError = recordingCursor.ReadPacket()

				now := time.Now().Unix()
				select {
				case <-motion:
					timestamp = now
					log.Log.Info("Recording expanding: motion detected while recording. Expanding recording.")
				default:
				}
				if timestamp+recordingPeriod-now <= 0 || now-startRecording >= maxRecordingPeriod {
					break
				}
				if pkt.IsKeyFrame {
					log.Log.Info("Recording writing: write frames")
					start = true
				}
				if start {
					if err := myMuxer.WritePacket(pkt); err != nil {
						log.Log.Error(err.Error())
					}
				}
			}

			// This will write the trailer as well.
			myMuxer.WriteTrailer()
			log.Log.Info("Recording finished: file save: " + name)
			file.Close()
			myMuxer = nil
			runtime.GC()
			debug.FreeOSMemory()

			// Check if need to convert to fragmented using bento
			if config.Capture.Fragmented == "true" && config.Capture.FragmentedDuration > 0 {
				CreateFragmentedMP4(fullName, config.Capture.FragmentedDuration)
			}

			// Create a symbol linc.
			fc, _ := os.Create("./data/cloud/" + name)
			fc.Close()
		}
	}

	log.Log.Debug("RecordStream: finished")
}
