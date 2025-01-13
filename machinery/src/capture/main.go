// Connecting to different camera sources and make it recording to disk.
package capture

import (
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"os"
	"strconv"
	"time"

	mp4ff "github.com/Eyevinn/mp4ff/mp4"
	"github.com/gin-gonic/gin"
	"github.com/kerberos-io/agent/machinery/src/conditions"
	"github.com/kerberos-io/agent/machinery/src/encryption"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/packets"
	"github.com/kerberos-io/agent/machinery/src/utils"
	"github.com/yapingcat/gomedia/go-mp4"
)

func CleanupRecordingDirectory(configDirectory string, configuration *models.Configuration) {
	autoClean := configuration.Config.AutoClean
	if autoClean == "true" {
		maxSize := configuration.Config.MaxDirectorySize
		if maxSize == 0 {
			maxSize = 300
		}
		// Total size of the recording directory.
		recordingsDirectory := configDirectory + "/data/recordings"
		size, err := utils.DirSize(recordingsDirectory)
		if err == nil {
			sizeInMB := size / 1000 / 1000
			if sizeInMB >= maxSize {
				// Remove the oldest recording
				oldestFile, err := utils.FindOldestFile(recordingsDirectory)
				if err == nil {
					err := os.Remove(recordingsDirectory + "/" + oldestFile.Name())
					log.Log.Info("HandleRecordStream: removed oldest file as part of cleanup - " + recordingsDirectory + "/" + oldestFile.Name())
					if err != nil {
						log.Log.Info("HandleRecordStream: something went wrong, " + err.Error())
					}
				} else {
					log.Log.Info("HandleRecordStream: something went wrong, " + err.Error())
				}
			}
		} else {
			log.Log.Info("HandleRecordStream: something went wrong, " + err.Error())
		}

	} else {
		log.Log.Info("HandleRecordStream: Autoclean disabled, nothing to do here.")
	}
}

func HandleRecordStream(queue *packets.Queue, configDirectory string, configuration *models.Configuration, communication *models.Communication, rtspClient RTSPClient) {

	config := configuration.Config
	loc, _ := time.LoadLocation(config.Timezone)

	if config.Capture.Recording == "false" {
		log.Log.Info("capture.main.HandleRecordStream(): disabled, we will not record anything.")
	} else {
		log.Log.Debug("capture.main.HandleRecordStream(): started")

		recordingPeriod := config.Capture.PostRecording         // number of seconds to record.
		maxRecordingPeriod := config.Capture.MaxLengthRecording // maximum number of seconds to record.

		// Synchronise the last synced time
		now := time.Now().Unix()
		startRecording := now
		timestamp := now

		if config.FriendlyName != "" {
			config.Name = config.FriendlyName
		}

		// For continuous and motion based recording we will use a single file.
		var file *os.File

		// Check if continuous recording.
		if config.Capture.Continuous == "true" {

			//var cws *cacheWriterSeeker
			var myMuxer *mp4.Movmuxer
			var videoTrack uint32
			var audioTrack uint32
			var name string

			// Do not do anything!
			log.Log.Info("capture.main.HandleRecordStream(continuous): start recording")

			now = time.Now().Unix()
			timestamp = now
			start := false

			// If continuous record the full length
			recordingPeriod = maxRecordingPeriod
			// Recording file name
			fullName := ""

			// Get as much packets we need.
			var cursorError error
			var pkt packets.Packet
			var nextPkt packets.Packet
			recordingStatus := "idle"
			recordingCursor := queue.Oldest()

			if cursorError == nil {
				pkt, cursorError = recordingCursor.ReadPacket()
			}

			fragmentSeqNr := 0
			var seg *mp4ff.MediaSegment
			var frag *mp4ff.Fragment
			var duration int64

			for cursorError == nil {

				nextPkt, cursorError = recordingCursor.ReadPacket()

				now := time.Now().Unix()

				if start && // If already recording and current frame is a keyframe and we should stop recording
					nextPkt.IsKeyFrame && (timestamp+recordingPeriod-now <= 0 || now-startRecording >= maxRecordingPeriod) {

					// Write the last packet
					ttime := convertPTS(pkt.TimeLegacy)
					if pkt.IsVideo {
						if err := myMuxer.Write(videoTrack, pkt.Data, ttime, ttime); err != nil {
							log.Log.Error("capture.main.HandleRecordStream(continuous): " + err.Error())
						}
					} else if pkt.IsAudio {
						if pkt.Codec == "AAC" {
							if err := myMuxer.Write(audioTrack, pkt.Data, ttime, ttime); err != nil {
								log.Log.Error("capture.main.HandleRecordStream(continuous): " + err.Error())
							}
						} else if pkt.Codec == "PCM_MULAW" {
							// TODO: transcode to AAC, some work to do..
							log.Log.Debug("capture.main.HandleRecordStream(continuous): no AAC audio codec detected, skipping audio track.")
						}
					}

					// This will write the trailer a well.
					if err := myMuxer.WriteTrailer(); err != nil {
						log.Log.Error("capture.main.HandleRecordStream(continuous): " + err.Error())
					}

					outPath := configDirectory + "/data/test/" + name
					appendToFile(seg, outPath)

					/*fragmentSeqNr++
					frag, _ := mp4ff.CreateFragment(uint32(fragmentSeqNr), mp4ff.DefaultTrakID)
					seg.AddFragment(frag)
					frag.AddFullSample(mp4ff.FullSample{
						Sample: mp4ff.Sample{
							Dur:  pkt.Packet.Timestamp,
							Size: uint32(len(pkt.Data)),
						},
						DecodeTime: uint64(pkt.Packet.Timestamp),
						Data:       pkt.Data,
					})

					outPath := configDirectory + "/data/test/" + name
					appendToFile(seg, outPath)

					ifd, err := os.Open(outPath)
					if err != nil {
						//return fmt.Errorf("could not open input file: %w", err)
					}
					defer ifd.Close()
					parsedMp4, err := mp4ff.DecodeFile(ifd, mp4ff.WithDecodeMode(mp4ff.DecModeNormal))
					fmt.Printf("parsedMp4: %+v\n", parsedMp4)
					if err != nil {
						fmt.Errorf("could not parse input file: %w", err)
					}

					w := os.Stdout
					err = parsedMp4.Info(w, "udta", "", "  ")
					if err != nil {
						fmt.Errorf("could not print info: %w", err)
					}
					fragmentSeqNr = 0*/

					log.Log.Info("capture.main.HandleRecordStream(continuous): recording finished: file save: " + name)

					// Cleanup muxer
					start = false
					file.Close()
					file = nil

					// Check if need to convert to fragmented using bento
					if config.Capture.Fragmented == "true" && config.Capture.FragmentedDuration > 0 {
						utils.CreateFragmentedMP4(fullName, config.Capture.FragmentedDuration)
					}

					// Check if we need to encrypt the recording.
					if config.Encryption != nil && config.Encryption.Enabled == "true" && config.Encryption.Recordings == "true" && config.Encryption.SymmetricKey != "" {
						// reopen file into memory 'fullName'
						contents, err := os.ReadFile(fullName)
						if err == nil {
							// encrypt
							encryptedContents, err := encryption.AesEncrypt(contents, config.Encryption.SymmetricKey)
							if err == nil {
								// write back to file
								err := os.WriteFile(fullName, []byte(encryptedContents), 0644)
								if err != nil {
									log.Log.Error("capture.main.HandleRecordStream(continuous): error writing file: " + err.Error())
								}
							} else {
								log.Log.Error("capture.main.HandleRecordStream(continuous): error encrypting file: " + err.Error())
							}
						} else {
							log.Log.Error("capture.main.HandleRecordStream(continuous): error reading file: " + err.Error())
						}
					}

					// Create a symbol link.
					fc, _ := os.Create(configDirectory + "/data/cloud/" + name)
					fc.Close()

					recordingStatus = "idle"

					// Clean up the recording directory if necessary.
					CleanupRecordingDirectory(configDirectory, configuration)
				}

				// If not yet started and a keyframe, let's make a recording
				if !start && pkt.IsKeyFrame {

					// We might have different conditions enabled such as time window or uri response.
					// We'll validate those conditions and if not valid we'll not do anything.
					valid, err := conditions.Validate(loc, configuration)
					if !valid && err != nil {
						log.Log.Debug("capture.main.HandleRecordStream(continuous): " + err.Error() + ".")
						time.Sleep(5 * time.Second)
						continue
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
					s := strconv.FormatInt(startRecording, 10) + "_" +
						"6" + "-" +
						"967003" + "_" +
						config.Name + "_" +
						"200-200-400-400" + "_0_" +
						"769"

					name = s + ".mp4"
					fullName = configDirectory + "/data/recordings/" + name

					// Running...
					log.Log.Info("capture.main.HandleRecordStream(continuous): recording started")

					file, err = os.Create(fullName)
					if err == nil {

						duration = 0
						streams, _ := rtspClient.GetVideoStreams()
						spsNALUs := [][]byte{streams[0].SPS}
						ppsNALUs := [][]byte{streams[0].PPS}

						videoTimescale := uint32(1000)
						init := mp4ff.CreateEmptyInit()
						init.AddEmptyTrack(videoTimescale, "video", "und")

						trak := init.Moov.Trak
						includePS := true
						err := trak.SetAVCDescriptor("avc1", spsNALUs, ppsNALUs, includePS)
						if err != nil {
							//return err
						}

						// Set duration
						init.Moov.Mvhd.Duration = uint64(20)
						init.Moov.Trak.Tkhd.Duration = uint64(20)
						init.Moov.Mvhd.CreationTime = uint64(time.Now().Unix())
						init.Moov.Trak.Tkhd.CreationTime = uint64(time.Now().Unix())

						width2 := trak.Mdia.Minf.Stbl.Stsd.AvcX.Width
						height2 := trak.Mdia.Minf.Stbl.Stsd.AvcX.Height
						fmt.Println("width: " + strconv.Itoa(int(width2)) + ", height: " + strconv.Itoa(int(height2)))

						// Add user data box
						udtaBox := mp4ff.UdtaBox{}
						freeBox := mp4ff.DataBox{
							Data: []byte("Fingerprint: xcxxx"),
						}
						udtaBox.AddChild(&freeBox)
						init.Moov.AddChild(&udtaBox)

						// Write to file
						outPath := configDirectory + "/data/test/" + name
						err = writeToFile(init, outPath)
						if err != nil {
						}

						seg = mp4ff.NewMediaSegment()

						//fragmentSeqNr++
						duration = 0
						frag, _ = mp4ff.CreateFragment(uint32(fragmentSeqNr), mp4ff.DefaultTrakID)
						seg.AddFragment(frag)
						frag.AddFullSample(mp4ff.FullSample{
							Sample: mp4ff.Sample{
								Dur:  uint32(30),
								Size: uint32(len(pkt.Data)),
							},
							DecodeTime: uint64(duration),
							Data:       pkt.Data,
						})

						//cws = newCacheWriterSeeker(4096)
						myMuxer, _ = mp4.CreateMp4Muxer(file)
						// We choose between H264 and H265
						width := configuration.Config.Capture.IPCamera.Width
						height := configuration.Config.Capture.IPCamera.Height
						widthOption := mp4.WithVideoWidth(uint32(width))
						heightOption := mp4.WithVideoHeight(uint32(height))
						if pkt.Codec == "H264" {
							videoTrack = myMuxer.AddVideoTrack(mp4.MP4_CODEC_H264, widthOption, heightOption)
						} else if pkt.Codec == "H265" {
							videoTrack = myMuxer.AddVideoTrack(mp4.MP4_CODEC_H265, widthOption, heightOption)
						}
						// For an MP4 container, AAC is the only audio codec supported.
						audioTrack = myMuxer.AddAudioTrack(mp4.MP4_CODEC_AAC)
					} else {
						log.Log.Error("capture.main.HandleRecordStream(continuous): " + err.Error())
					}

					ttime := convertPTS(pkt.TimeLegacy)
					if pkt.IsVideo {
						if err := myMuxer.Write(videoTrack, pkt.Data, ttime, ttime); err != nil {
							log.Log.Error("capture.main.HandleRecordStream(continuous): " + err.Error())
						}

						/*fragmentSeqNr++
						frag, _ = mp4ff.CreateFragment(uint32(fragmentSeqNr), mp4ff.DefaultTrakID)
						seg.AddFragment(frag)
						frag.AddFullSample(mp4ff.FullSample{
							Sample: mp4ff.Sample{
								Dur:  pkt.Packet.Timestamp,
								Size: uint32(len(pkt.Data)),
							},
							DecodeTime: uint64(pkt.Packet.Timestamp),
							Data:       pkt.Data,
						})*/

					} else if pkt.IsAudio {
						if pkt.Codec == "AAC" {
							if err := myMuxer.Write(audioTrack, pkt.Data, ttime, ttime); err != nil {
								log.Log.Error("capture.main.HandleRecordStream(continuous): " + err.Error())
							}
						} else if pkt.Codec == "PCM_MULAW" {
							// TODO: transcode to AAC, some work to do..
							log.Log.Debug("capture.main.HandleRecordStream(continuous): no AAC audio codec detected, skipping audio track.")
						}
					}

					recordingStatus = "started"

				} else if start {
					ttime := convertPTS(pkt.TimeLegacy)
					if pkt.IsVideo {
						if err := myMuxer.Write(videoTrack, pkt.Data, ttime, ttime); err != nil {
							log.Log.Error("capture.main.HandleRecordStream(continuous): " + err.Error())
						}

						/*if pkt.IsKeyFrame {
							duration = duration + 30
							frag.AddFullSample(mp4ff.FullSample{
								Sample: mp4ff.Sample{
									Dur:  uint32(30),
									Size: uint32(len(pkt.Data)),
								},
								DecodeTime: uint64(duration),
								Data:       pkt.Data,
							})
						}*/

						/*fragmentSeqNr++
						frag, _ := mp4ff.CreateFragment(uint32(fragmentSeqNr), mp4ff.DefaultTrakID)
						seg.AddFragment(frag)
						frag.AddFullSample(mp4ff.FullSample{
							Sample: mp4ff.Sample{
								Dur:  pkt.Packet.Timestamp,
								Size: uint32(len(pkt.Data)),
							},
							DecodeTime: uint64(pkt.Packet.Timestamp),
							Data:       pkt.Data,
						})*/
					} else if pkt.IsAudio {
						if pkt.Codec == "AAC" {
							if err := myMuxer.Write(audioTrack, pkt.Data, ttime, ttime); err != nil {
								log.Log.Error("capture.main.HandleRecordStream(continuous): " + err.Error())
							}
						} else if pkt.Codec == "PCM_MULAW" {
							// TODO: transcode to AAC, some work to do..
							log.Log.Debug("capture.main.HandleRecordStream(continuous): no AAC audio codec detected, skipping audio track.")
						}
					}
				}

				pkt = nextPkt
			}

			// We might have interrupted the recording while restarting the agent.
			// If this happens we need to check to properly close the recording.
			if cursorError != nil {
				if recordingStatus == "started" {
					// This will write the trailer a well.
					if err := myMuxer.WriteTrailer(); err != nil {
						log.Log.Error(err.Error())
					}

					log.Log.Info("capture.main.HandleRecordStream(continuous): Recording finished: file save: " + name)

					// Cleanup muxer
					start = false
					file.Close()
					file = nil

					fragmentSeqNr = 0

					// Check if need to convert to fragmented using bento
					if config.Capture.Fragmented == "true" && config.Capture.FragmentedDuration > 0 {
						utils.CreateFragmentedMP4(fullName, config.Capture.FragmentedDuration)
					}

					// Check if we need to encrypt the recording.
					if config.Encryption != nil && config.Encryption.Enabled == "true" && config.Encryption.Recordings == "true" && config.Encryption.SymmetricKey != "" {
						// reopen file into memory 'fullName'
						contents, err := os.ReadFile(fullName)
						if err == nil {
							// encrypt
							encryptedContents, err := encryption.AesEncrypt(contents, config.Encryption.SymmetricKey)
							if err == nil {
								// write back to file
								err := os.WriteFile(fullName, []byte(encryptedContents), 0644)
								if err != nil {
									log.Log.Error("capture.main.HandleRecordStream(motiondetection): error writing file: " + err.Error())
								}
							} else {
								log.Log.Error("capture.main.HandleRecordStream(motiondetection): error encrypting file: " + err.Error())
							}
						} else {
							log.Log.Error("capture.main.HandleRecordStream(motiondetection): error reading file: " + err.Error())
						}
					}

					// Create a symbol link.
					fc, _ := os.Create(configDirectory + "/data/cloud/" + name)
					fc.Close()

					recordingStatus = "idle"

					// Clean up the recording directory if necessary.
					CleanupRecordingDirectory(configDirectory, configuration)
				}
			}
		} else {

			log.Log.Info("capture.main.HandleRecordStream(motiondetection): Start motion based recording ")

			var lastDuration int64
			var lastRecordingTime int64

			//var cws *cacheWriterSeeker
			var myMuxer *mp4.Movmuxer
			var videoTrack uint32
			var audioTrack uint32

			for motion := range communication.HandleMotion {

				timestamp = time.Now().Unix()
				startRecording = time.Now().Unix() // we mark the current time when the record started.
				numberOfChanges := motion.NumberOfChanges

				// If we have prerecording we will substract the number of seconds.
				// Taking into account FPS = GOP size (Keyfram interval)
				if config.Capture.PreRecording > 0 {

					// Might be that recordings are coming short after each other.
					// Therefore we do some math with the current time and the last recording time.

					timeBetweenNowAndLastRecording := startRecording - lastRecordingTime
					if timeBetweenNowAndLastRecording > int64(config.Capture.PreRecording) {
						startRecording = startRecording - int64(config.Capture.PreRecording) + 1
					} else {
						startRecording = startRecording - timeBetweenNowAndLastRecording
					}
				}

				// timestamp_microseconds_instanceName_regionCoordinates_numberOfChanges_token
				// 1564859471_6-474162_oprit_577-283-727-375_1153_27.mp4
				// - Timestamp
				// - Size + - + microseconds
				// - device
				// - Region
				// - Number of changes
				// - Token

				s := strconv.FormatInt(startRecording, 10) + "_" +
					"6" + "-" +
					"967003" + "_" +
					config.Name + "_" +
					"200-200-400-400" + "_" +
					strconv.Itoa(numberOfChanges) + "_" +
					"769"

				name := s + ".mp4"
				fullName := configDirectory + "/data/recordings/" + name

				// Running...
				log.Log.Info("capture.main.HandleRecordStream(motiondetection): recording started")
				file, _ = os.Create(fullName)
				myMuxer, _ = mp4.CreateMp4Muxer(file)

				// Check which video codec we need to use.
				videoSteams, _ := rtspClient.GetVideoStreams()
				for _, stream := range videoSteams {
					width := configuration.Config.Capture.IPCamera.Width
					height := configuration.Config.Capture.IPCamera.Height
					widthOption := mp4.WithVideoWidth(uint32(width))
					heightOption := mp4.WithVideoHeight(uint32(height))
					if stream.Name == "H264" {
						videoTrack = myMuxer.AddVideoTrack(mp4.MP4_CODEC_H264, widthOption, heightOption)
					} else if stream.Name == "H265" {
						videoTrack = myMuxer.AddVideoTrack(mp4.MP4_CODEC_H265, widthOption, heightOption)
					}
				}
				// For an MP4 container, AAC is the only audio codec supported.
				audioTrack = myMuxer.AddAudioTrack(mp4.MP4_CODEC_AAC)
				start := false

				// Get as much packets we need.
				var cursorError error
				var pkt packets.Packet
				var nextPkt packets.Packet
				recordingCursor := queue.DelayedGopCount(int(config.Capture.PreRecording + 1))

				if cursorError == nil {
					pkt, cursorError = recordingCursor.ReadPacket()
				}

				for cursorError == nil {

					nextPkt, cursorError = recordingCursor.ReadPacket()
					if cursorError != nil {
						log.Log.Error("capture.main.HandleRecordStream(motiondetection): " + cursorError.Error())
					}

					now := time.Now().Unix()
					select {
					case motion := <-communication.HandleMotion:
						timestamp = now
						log.Log.Info("capture.main.HandleRecordStream(motiondetection): motion detected while recording. Expanding recording.")
						numberOfChanges = motion.NumberOfChanges
						log.Log.Info("capture.main.HandleRecordStream(motiondetection): Received message with recording data, detected changes to save: " + strconv.Itoa(numberOfChanges))
					default:
					}

					if (timestamp+recordingPeriod-now < 0 || now-startRecording > maxRecordingPeriod) && nextPkt.IsKeyFrame {
						log.Log.Info("capture.main.HandleRecordStream(motiondetection): closing recording (timestamp: " + strconv.FormatInt(timestamp, 10) + ", recordingPeriod: " + strconv.FormatInt(recordingPeriod, 10) + ", now: " + strconv.FormatInt(now, 10) + ", startRecording: " + strconv.FormatInt(startRecording, 10) + ", maxRecordingPeriod: " + strconv.FormatInt(maxRecordingPeriod, 10))
						break
					}
					if pkt.IsKeyFrame && !start && pkt.Time >= lastDuration {
						log.Log.Debug("capture.main.HandleRecordStream(motiondetection): write frames")
						start = true
					}
					if start {
						ttime := convertPTS(pkt.TimeLegacy)
						if pkt.IsVideo {
							if err := myMuxer.Write(videoTrack, pkt.Data, ttime, ttime); err != nil {
								log.Log.Error("capture.main.HandleRecordStream(motiondetection): " + err.Error())
							}
						} else if pkt.IsAudio {
							if pkt.Codec == "AAC" {
								if err := myMuxer.Write(audioTrack, pkt.Data, ttime, ttime); err != nil {
									log.Log.Error("capture.main.HandleRecordStream(motiondetection): " + err.Error())
								}
							} else if pkt.Codec == "PCM_MULAW" {
								// TODO: transcode to AAC, some work to do..
								log.Log.Debug("capture.main.HandleRecordStream(motiondetection): no AAC audio codec detected, skipping audio track.")
							}
						}

						// We will sync to file every keyframe.
						if pkt.IsKeyFrame {
							err := file.Sync()
							if err != nil {
								log.Log.Error("capture.main.HandleRecordStream(motiondetection): " + err.Error())
							} else {
								log.Log.Debug("capture.main.HandleRecordStream(motiondetection): synced file " + name)
							}
						}
					}

					pkt = nextPkt
				}

				// This will write the trailer a well.
				myMuxer.WriteTrailer()

				log.Log.Info("capture.main.HandleRecordStream(motiondetection): file save: " + name)

				lastDuration = pkt.Time
				lastRecordingTime = time.Now().Unix()
				file.Close()
				file = nil

				// Check if need to convert to fragmented using bento
				if config.Capture.Fragmented == "true" && config.Capture.FragmentedDuration > 0 {
					utils.CreateFragmentedMP4(fullName, config.Capture.FragmentedDuration)
				}

				// Check if we need to encrypt the recording.
				if config.Encryption != nil && config.Encryption.Enabled == "true" && config.Encryption.Recordings == "true" && config.Encryption.SymmetricKey != "" {
					// reopen file into memory 'fullName'
					contents, err := os.ReadFile(fullName)
					if err == nil {
						// encrypt
						encryptedContents, err := encryption.AesEncrypt(contents, config.Encryption.SymmetricKey)
						if err == nil {
							// write back to file
							err := os.WriteFile(fullName, []byte(encryptedContents), 0644)
							if err != nil {
								log.Log.Error("capture.main.HandleRecordStream(motiondetection): error writing file: " + err.Error())
							}
						} else {
							log.Log.Error("capture.main.HandleRecordStream(motiondetection): error encrypting file: " + err.Error())
						}
					} else {
						log.Log.Error("capture.main.HandleRecordStream(motiondetection): error reading file: " + err.Error())
					}
				}

				// Create a symbol linc.
				fc, _ := os.Create(configDirectory + "/data/cloud/" + name)
				fc.Close()

				// Clean up the recording directory if necessary.
				CleanupRecordingDirectory(configDirectory, configuration)
			}
		}

		log.Log.Debug("capture.main.HandleRecordStream(): finished")
	}
}

// VerifyCamera godoc
// @Router /api/camera/verify/{streamType} [post]
// @ID verify-camera
// @Tags camera
// @Param streamType path string true "Stream Type" Enums(primary, secondary)
// @Param cameraStreams body models.CameraStreams true "Camera Streams"
// @Summary Validate a specific RTSP profile camera connection.
// @Description This method will validate a specific profile connection from an RTSP camera, and try to get the codec.
// @Success 200 {object} models.APIResponse
func VerifyCamera(c *gin.Context) {

	var cameraStreams models.CameraStreams
	err := c.BindJSON(&cameraStreams)

	// Should return in 5 seconds.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err == nil {

		streamType := c.Param("streamType")
		if streamType == "" {
			streamType = "primary"
		}

		rtspUrl := cameraStreams.RTSP
		if streamType == "secondary" {
			rtspUrl = cameraStreams.SubRTSP
		}

		// Currently only support H264 encoded cameras, this will change.
		// Establishing the camera connection without backchannel if no substream
		rtspClient := &Golibrtsp{
			Url: rtspUrl,
		}

		err := rtspClient.Connect(ctx)
		if err == nil {

			// Get the streams from the rtsp client.
			streams, _ := rtspClient.GetStreams()

			videoIdx := -1
			audioIdx := -1
			for i, stream := range streams {
				if (stream.Name == "H264" || stream.Name == "H265") && videoIdx < 0 {
					videoIdx = i
				} else if stream.Name == "PCM_MULAW" && audioIdx < 0 {
					audioIdx = i
				}
			}

			err := rtspClient.Close()
			if err == nil {
				if videoIdx > -1 {
					c.JSON(200, models.APIResponse{
						Message: "All good, detected a H264 codec.",
						Data:    streams,
					})
				} else {
					c.JSON(400, models.APIResponse{
						Message: "Stream doesn't have a H264 codec, we only support H264 so far.",
					})
				}
			} else {
				c.JSON(400, models.APIResponse{
					Message: "Something went wrong while closing the connection " + err.Error(),
				})
			}
		} else {
			c.JSON(400, models.APIResponse{
				Message: err.Error(),
			})
		}
	} else {
		c.JSON(400, models.APIResponse{
			Message: "Something went wrong while receiving the config " + err.Error(),
		})
	}
}

func Base64Image(captureDevice *Capture, communication *models.Communication) string {
	// We'll try to get a snapshot from the camera.
	var queue *packets.Queue
	var cursor *packets.QueueCursor

	// We'll pick the right client and decoder.
	rtspClient := captureDevice.RTSPSubClient
	if rtspClient != nil {
		queue = communication.SubQueue
		cursor = queue.Latest()
	} else {
		rtspClient = captureDevice.RTSPClient
		queue = communication.Queue
		cursor = queue.Latest()
	}

	// We'll try to have a keyframe, if not we'll return an empty string.
	var encodedImage string
	// Try for 3 times in a row.
	count := 0
	for count < 3 {
		if queue != nil && cursor != nil && rtspClient != nil {
			pkt, err := cursor.ReadPacket()
			if err == nil {
				if !pkt.IsKeyFrame {
					continue
				}
				var img image.YCbCr
				img, err = (*rtspClient).DecodePacket(pkt)
				if err == nil {
					bytes, _ := utils.ImageToBytes(&img)
					encodedImage = base64.StdEncoding.EncodeToString(bytes)
					break
				} else {
					count++
					continue
				}
			}
		} else {
			break
		}
	}
	return encodedImage
}

func JpegImage(captureDevice *Capture, communication *models.Communication) image.YCbCr {
	// We'll try to get a snapshot from the camera.
	var queue *packets.Queue
	var cursor *packets.QueueCursor

	// We'll pick the right client and decoder.
	rtspClient := captureDevice.RTSPSubClient
	if rtspClient != nil {
		queue = communication.SubQueue
		cursor = queue.Latest()
	} else {
		rtspClient = captureDevice.RTSPClient
		queue = communication.Queue
		cursor = queue.Latest()
	}

	// We'll try to have a keyframe, if not we'll return an empty string.
	var image image.YCbCr
	// Try for 3 times in a row.
	count := 0
	for count < 3 {
		if queue != nil && cursor != nil && rtspClient != nil {
			pkt, err := cursor.ReadPacket()
			if err == nil {
				if !pkt.IsKeyFrame {
					continue
				}
				image, err = (*rtspClient).DecodePacket(pkt)
				if err != nil {
					count++
					continue
				} else {
					break
				}
			}
		} else {
			break
		}
	}
	return image
}

func convertPTS(v time.Duration) uint64 {
	return uint64(v.Milliseconds())
}

func convertPTS2(v int64) uint64 {
	return uint64(v) / 100
}

func writeToFile(init *mp4ff.InitSegment, filePath string) error {
	// Next write to a file
	ofd, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer ofd.Close()
	err = init.Encode(ofd)
	return err
}

func appendToFile(init *mp4ff.MediaSegment, filePath string) error {
	// Next write to a file
	ofd, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer ofd.Close()
	err = init.Encode(ofd)
	return err
}
