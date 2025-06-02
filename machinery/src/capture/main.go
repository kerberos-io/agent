// Connecting to different camera sources and make it recording to disk.
package capture

import (
	"context"
	"encoding/base64"
	"image"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kerberos-io/agent/machinery/src/conditions"
	"github.com/kerberos-io/agent/machinery/src/encryption"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/packets"
	"github.com/kerberos-io/agent/machinery/src/utils"
	"github.com/kerberos-io/agent/machinery/src/video"
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
			var mp4Video *video.MP4
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

			for cursorError == nil {

				nextPkt, cursorError = recordingCursor.ReadPacket()

				now := time.Now().Unix()

				if start && // If already recording and current frame is a keyframe and we should stop recording
					nextPkt.IsKeyFrame && (timestamp+recordingPeriod-now <= 0 || now-startRecording >= maxRecordingPeriod) {

					// Write the last packet
					ttimeLegacy := convertPTS(pkt.TimeLegacy)
					ttime := convertPTS2(pkt.Time)
					ttimeNext := convertPTS2(nextPkt.Time)
					duration := ttimeNext - ttime

					if pkt.IsVideo {

						// New method using new mp4 library
						mp4Video.AddSampleToTrack(1, pkt.IsKeyFrame, pkt.Data, ttime, duration)

						if err := myMuxer.Write(videoTrack, pkt.Data, ttimeLegacy, ttimeLegacy); err != nil {
							log.Log.Error("capture.main.HandleRecordStream(continuous): " + err.Error())
						}
					} else if pkt.IsAudio {
						if pkt.Codec == "AAC" {
							if err := myMuxer.Write(audioTrack, pkt.Data, ttimeLegacy, ttimeLegacy); err != nil {
								log.Log.Error("capture.main.HandleRecordStream(continuous): " + err.Error())
							}
						} else if pkt.Codec == "PCM_MULAW" {
							// TODO: transcode to AAC, some work to do..
							log.Log.Debug("capture.main.HandleRecordStream(continuous): no AAC audio codec detected, skipping audio track.")
						}
					}
					// Close mp4
					mp4Video.Close()

					// This will write the trailer a well.
					if err := myMuxer.WriteTrailer(); err != nil {
						log.Log.Error("capture.main.HandleRecordStream(continuous): " + err.Error())
					}

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
					new_name := s + "_new.mp4"
					fullName = configDirectory + "/data/recordings/" + new_name

					new_fullName := configDirectory + "/data/recordings/" + name

					// Running...
					log.Log.Info("capture.main.HandleRecordStream(continuous): recording started")
					file, err = os.Create(fullName)

					if err == nil {

						// Get width and height from the camera.
						width := configuration.Config.Capture.IPCamera.Width
						height := configuration.Config.Capture.IPCamera.Height

						// Get SPS and PPS NALUs from the camera.
						spsNALUS := configuration.Config.Capture.IPCamera.SPSNALUs
						ppsNALUS := configuration.Config.Capture.IPCamera.PPSNALUs

						// Create a video file, and set the dimensions.
						mp4Video = video.NewMP4(new_fullName, spsNALUS, ppsNALUS)
						mp4Video.SetWidth(width)
						mp4Video.SetHeight(height)
						//mp4Video.AddVideoTrack("H264")
						//mp4Video.AddAudioTrack("AAC")

						myMuxer, _ = mp4.CreateMp4Muxer(file)
						// We choose between H264 and H265
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

					ttimeLegacy := convertPTS(pkt.TimeLegacy)
					ttime := convertPTS2(pkt.Time)
					ttimeNext := convertPTS2(nextPkt.Time)
					duration := ttimeNext - ttime

					if pkt.IsVideo {
						// New method using new mp4 library
						mp4Video.AddSampleToTrack(1, pkt.IsKeyFrame, pkt.Data, ttime, duration)

						if err := myMuxer.Write(videoTrack, pkt.Data, ttimeLegacy, ttimeLegacy); err != nil {
							log.Log.Error("capture.main.HandleRecordStream(continuous): " + err.Error())
						}
					} else if pkt.IsAudio {
						if pkt.Codec == "AAC" {
							if err := myMuxer.Write(audioTrack, pkt.Data, ttimeLegacy, ttimeLegacy); err != nil {
								log.Log.Error("capture.main.HandleRecordStream(continuous): " + err.Error())
							}
						} else if pkt.Codec == "PCM_MULAW" {
							// TODO: transcode to AAC, some work to do..
							log.Log.Debug("capture.main.HandleRecordStream(continuous): no AAC audio codec detected, skipping audio track.")
						}
					}
					recordingStatus = "started"

				} else if start {

					ttimeLegacy := convertPTS(pkt.TimeLegacy)
					ttime := convertPTS2(pkt.Time)
					ttimeNext := convertPTS2(nextPkt.Time)
					duration := ttimeNext - ttime

					if pkt.IsVideo {
						// New method using new mp4 library
						mp4Video.AddSampleToTrack(1, pkt.IsKeyFrame, pkt.Data, ttime, duration)

						if err := myMuxer.Write(videoTrack, pkt.Data, ttimeLegacy, ttimeLegacy); err != nil {
							log.Log.Error("capture.main.HandleRecordStream(continuous): " + err.Error())
						}
					} else if pkt.IsAudio {
						if pkt.Codec == "AAC" {
							if err := myMuxer.Write(audioTrack, pkt.Data, ttimeLegacy, ttimeLegacy); err != nil {
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
