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
	"go.opentelemetry.io/otel/trace"
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

		preRecording := config.Capture.PreRecording * 1000
		postRecording := config.Capture.PostRecording * 1000           // number of seconds to record.
		maxRecordingPeriod := config.Capture.MaxLengthRecording * 1000 // maximum number of seconds to record.

		// We will calculate the maxRecordingPeriod based on the preRecording and postRecording values.
		if maxRecordingPeriod == 0 {
			// If maxRecordingPeriod is not set, we will use the preRecording and postRecording values
			maxRecordingPeriod = preRecording + postRecording
		}

		if maxRecordingPeriod < preRecording+postRecording {
			log.Log.Error("capture.main.HandleRecordStream(): maxRecordingPeriod is less than preRecording + postRecording, this is not allowed. Setting maxRecordingPeriod to preRecording + postRecording.")
			maxRecordingPeriod = preRecording + postRecording
		}

		if config.FriendlyName != "" {
			config.Name = config.FriendlyName
		}

		// Get the audio and video codec from the camera.
		// We only expect one audio and one video codec.
		// If there are multiple audio or video streams, we will use the first one.
		audioCodec := ""
		videoCodec := ""
		audioStreams, _ := rtspClient.GetAudioStreams()
		videoStreams, _ := rtspClient.GetVideoStreams()
		if len(audioStreams) > 0 {
			audioCodec = audioStreams[0].Name
			config.Capture.IPCamera.SampleRate = audioStreams[0].SampleRate
			config.Capture.IPCamera.Channels = audioStreams[0].Channels
		}
		if len(videoStreams) > 0 {
			videoCodec = videoStreams[0].Name
		}

		// Check if continuous recording.
		if config.Capture.Continuous == "true" {

			//var cws *cacheWriterSeeker
			var mp4Video *video.MP4
			var videoTrack uint32
			var audioTrack uint32
			var name string

			// Do not do anything!
			log.Log.Info("capture.main.HandleRecordStream(continuous): start recording")

			start := false

			// If continuous record the full length
			postRecording = maxRecordingPeriod
			// Recording file name
			fullName := ""

			var startRecording int64 = 0 // start recording timestamp in milliseconds

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

				now := time.Now().UnixMilli()

				if start && // If already recording and current frame is a keyframe and we should stop recording
					nextPkt.IsKeyFrame && (startRecording+postRecording-now <= 0 || now-startRecording > maxRecordingPeriod-500) {

					pts := convertPTS(pkt.TimeLegacy)
					if pkt.IsVideo {
						// Write the last packet
						if err := mp4Video.AddSampleToTrack(videoTrack, pkt.IsKeyFrame, pkt.Data, pts); err != nil {
							log.Log.Error("capture.main.HandleRecordStream(continuous): " + err.Error())
						}
					} else if pkt.IsAudio {
						// Write the last packet
						if pkt.Codec == "AAC" {
							if err := mp4Video.AddSampleToTrack(audioTrack, pkt.IsKeyFrame, pkt.Data, pts); err != nil {
								log.Log.Error("capture.main.HandleRecordStream(continuous): " + err.Error())
							}
						} else if pkt.Codec == "PCM_MULAW" {
							// TODO: transcode to AAC, some work to do..
							log.Log.Debug("capture.main.HandleRecordStream(continuous): no AAC audio codec detected, skipping audio track.")
						}
					}

					// Close mp4
					if len(mp4Video.SPSNALUs) == 0 && len(configuration.Config.Capture.IPCamera.SPSNALUs) > 0 {
						mp4Video.SPSNALUs = configuration.Config.Capture.IPCamera.SPSNALUs
					}
					if len(mp4Video.PPSNALUs) == 0 && len(configuration.Config.Capture.IPCamera.PPSNALUs) > 0 {
						mp4Video.PPSNALUs = configuration.Config.Capture.IPCamera.PPSNALUs
					}
					if len(mp4Video.VPSNALUs) == 0 && len(configuration.Config.Capture.IPCamera.VPSNALUs) > 0 {
						mp4Video.VPSNALUs = configuration.Config.Capture.IPCamera.VPSNALUs
					}
					if (videoCodec == "H264" && (len(mp4Video.SPSNALUs) == 0 || len(mp4Video.PPSNALUs) == 0)) ||
						(videoCodec == "H265" && (len(mp4Video.VPSNALUs) == 0 || len(mp4Video.SPSNALUs) == 0 || len(mp4Video.PPSNALUs) == 0)) {
						log.Log.Warning("capture.main.HandleRecordStream(continuous): closing MP4 without full parameter sets, moov may be incomplete")
					}
					mp4Video.Close(&config)
					log.Log.Info("capture.main.HandleRecordStream(continuous): recording finished: file save: " + name)

					// Cleanup muxer
					start = false

					// Update the name of the recording with the duration.
					// We will update the name of the recording with the duration in milliseconds.
					if mp4Video.VideoTotalDuration > 0 {
						duration := mp4Video.VideoTotalDuration
						// Update the name with the duration in milliseconds.
						startRecordingSeconds := startRecording / 1000      // convert to seconds
						startRecordingMilliseconds := startRecording % 1000 // convert to milliseconds
						s := strconv.FormatInt(startRecordingSeconds, 10) + "_" +
							strconv.Itoa(len(strconv.FormatInt(startRecordingMilliseconds, 10))) + "-" +
							strconv.FormatInt(startRecordingMilliseconds, 10) + "_" +
							config.Name + "_" +
							"0-0-0-0" + "_" + // region coordinates, we
							"-1" + "_" + // token
							strconv.FormatInt(int64(duration), 10) // + "_" + // duration of recording
							//utils.VERSION // version of the agent

						oldName := name
						name = s + ".mp4"
						fullName = configDirectory + "/data/recordings/" + name
						log.Log.Info("capture.main.HandleRecordStream(motiondetection): renamed file from: " + oldName + " to: " + name)

						// Rename the file to the new name.
						err := os.Rename(
							configDirectory+"/data/recordings/"+oldName,
							configDirectory+"/data/recordings/"+s+".mp4")

						if err != nil {
							log.Log.Error("capture.main.HandleRecordStream(motiondetection): error renaming file: " + err.Error())
						}
					} else {
						log.Log.Info("capture.main.HandleRecordStream(continuous): no video data recorded, not renaming file.")
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

					// timestamp_microseconds_instanceName_regionCoordinates_numberOfChanges_token
					// 1564859471_6-474162_oprit_577-283-727-375_1153_27.mp4
					// - Timestamp
					// - Size + - + microseconds
					// - device
					// - Region
					// - Number of changes
					// - Token

					startRecording = pkt.CurrentTime
					startRecordingSeconds := startRecording / 1000            // convert to seconds
					startRecordingMilliseconds := startRecording % 1000       // convert to milliseconds
					s := strconv.FormatInt(startRecordingSeconds, 10) + "_" + // start timestamp in seconds
						strconv.Itoa(len(strconv.FormatInt(startRecordingMilliseconds, 10))) + "-" + // length of milliseconds
						strconv.FormatInt(startRecordingMilliseconds, 10) + "_" + // milliseconds
						config.Name + "_" + // device name
						"0-0-0-0" + "_" + // region coordinates, we will not use this for continuous recording
						"0" + "_" + // token
						"0" + "_" //+ // duration of recording in milliseconds
					//utils.VERSION // version of the agent

					name = s + ".mp4"
					fullName = configDirectory + "/data/recordings/" + name

					// Running...
					log.Log.Info("capture.main.HandleRecordStream(continuous): recording started")

					// Get width and height from the camera.
					width := configuration.Config.Capture.IPCamera.Width
					height := configuration.Config.Capture.IPCamera.Height

					// Get SPS and PPS NALUs from the camera.
					spsNALUS := configuration.Config.Capture.IPCamera.SPSNALUs
					ppsNALUS := configuration.Config.Capture.IPCamera.PPSNALUs
					vpsNALUS := configuration.Config.Capture.IPCamera.VPSNALUs

					if len(spsNALUS) == 0 || len(ppsNALUS) == 0 {
						log.Log.Warning("capture.main.HandleRecordStream(continuous): missing SPS/PPS at recording start")
					}
					// Create a video file, and set the dimensions.
					mp4Video = video.NewMP4(fullName, spsNALUS, ppsNALUS, vpsNALUS, configuration.Config.Capture.MaxLengthRecording)
					mp4Video.SetWidth(width)
					mp4Video.SetHeight(height)

					if videoCodec == "H264" {
						videoTrack = mp4Video.AddVideoTrack("H264")
					} else if videoCodec == "H265" {
						videoTrack = mp4Video.AddVideoTrack("H265")
					}
					if audioCodec == "AAC" {
						audioTrack = mp4Video.AddAudioTrack("AAC")
					} else if audioCodec == "PCM_MULAW" {
						log.Log.Debug("capture.main.HandleRecordStream(continuous): no AAC audio codec detected, skipping audio track.")
					}

					pts := convertPTS(pkt.TimeLegacy)
					if pkt.IsVideo {
						if err := mp4Video.AddSampleToTrack(videoTrack, pkt.IsKeyFrame, pkt.Data, pts); err != nil {
							log.Log.Error("capture.main.HandleRecordStream(continuous): " + err.Error())
						}
					} else if pkt.IsAudio {
						if pkt.Codec == "AAC" {
							if err := mp4Video.AddSampleToTrack(audioTrack, pkt.IsKeyFrame, pkt.Data, pts); err != nil {
								log.Log.Error("capture.main.HandleRecordStream(continuous): " + err.Error())
							}
						} else if pkt.Codec == "PCM_MULAW" {
							// TODO: transcode to AAC, some work to do..
							// We might need to use ffmpeg to transcode the audio to AAC.
							// For now we will skip the audio track.
							log.Log.Debug("capture.main.HandleRecordStream(continuous): no AAC audio codec detected, skipping audio track.")
						}
					}
					recordingStatus = "started"

				} else if start {

					pts := convertPTS(pkt.TimeLegacy)
					if pkt.IsVideo {
						// New method using new mp4 library
						if err := mp4Video.AddSampleToTrack(videoTrack, pkt.IsKeyFrame, pkt.Data, pts); err != nil {
							log.Log.Error("capture.main.HandleRecordStream(continuous): " + err.Error())
						}
					} else if pkt.IsAudio {
						if pkt.Codec == "AAC" {
							if err := mp4Video.AddSampleToTrack(audioTrack, pkt.IsKeyFrame, pkt.Data, pts); err != nil {
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

					log.Log.Info("capture.main.HandleRecordStream(continuous): Recording finished: file save: " + name)

					// Cleanup muxer
					start = false

					// Update the name of the recording with the duration.
					// We will update the name of the recording with the duration in milliseconds.
					if mp4Video.VideoTotalDuration > 0 {
						duration := mp4Video.VideoTotalDuration
						// Update the name with the duration in milliseconds.
						startRecordingSeconds := startRecording / 1000      // convert to seconds
						startRecordingMilliseconds := startRecording % 1000 // convert to milliseconds
						s := strconv.FormatInt(startRecordingSeconds, 10) + "_" +
							strconv.Itoa(len(strconv.FormatInt(startRecordingMilliseconds, 10))) + "-" +
							strconv.FormatInt(startRecordingMilliseconds, 10) + "_" +
							config.Name + "_" +
							"0-0-0-0" + "_" + // region coordinates, we
							"-1" + "_" + // token
							strconv.FormatInt(int64(duration), 10) // + "_" + // duration of recording
							//utils.VERSION // version of the agent

						oldName := name
						name = s + ".mp4"
						fullName = configDirectory + "/data/recordings/" + name
						log.Log.Info("capture.main.HandleRecordStream(motiondetection): renamed file from: " + oldName + " to: " + name)

						// Rename the file to the new name.
						err := os.Rename(
							configDirectory+"/data/recordings/"+oldName,
							configDirectory+"/data/recordings/"+s+".mp4")

						if err != nil {
							log.Log.Error("capture.main.HandleRecordStream(motiondetection): error renaming file: " + err.Error())
						}
					} else {
						log.Log.Info("capture.main.HandleRecordStream(continuous): no video data recorded, not renaming file.")
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

			var lastRecordingTime int64 = 0 // last recording timestamp in milliseconds
			var displayTime int64 = 0       // display time in milliseconds

			var videoTrack uint32
			var audioTrack uint32

			for motion := range communication.HandleMotion {

				// Get as much packets we need.
				var cursorError error
				var pkt packets.Packet
				var nextPkt packets.Packet
				recordingCursor := queue.Oldest() // Start from the latest packet in the queue)

				now := time.Now().UnixMilli()
				motionTimestamp := now

				start := false

				if cursorError == nil {
					pkt, cursorError = recordingCursor.ReadPacket()
				}

				displayTime = pkt.CurrentTime
				startRecording := pkt.CurrentTime

				// We have more packets in the queue (which might still be older than where we close the previous recording).
				// In that case we will use the last recording time to determine the start time of the recording, otherwise
				// we will have duplicate frames in the recording.
				if startRecording < lastRecordingTime {
					displayTime = lastRecordingTime
					startRecording = lastRecordingTime
				}

				// If startRecording is 0, we will continue as it might be we are in a state of restarting the agent.
				if startRecording == 0 {
					log.Log.Info("capture.main.HandleRecordStream(motiondetection): startRecording is 0, we will continue as it might be we are in a state of restarting the agent.")
					continue
				}

				// timestamp_microseconds_instanceName_regionCoordinates_numberOfChanges_token
				// 1564859471_6-474162_oprit_577-283-727-375_1153_27.mp4
				// - Timestamp
				// - Size + - + microseconds
				// - device
				// - Region
				// - Number of changes
				// - Token

				displayTimeSeconds := displayTime / 1000      // convert to seconds
				displayTimeMilliseconds := displayTime % 1000 // convert to milliseconds
				motionRectangleString := "0-0-0-0"
				if motion.Rectangle.X != 0 || motion.Rectangle.Y != 0 ||
					motion.Rectangle.Width != 0 || motion.Rectangle.Height != 0 {
					motionRectangleString = strconv.Itoa(motion.Rectangle.X) + "-" + strconv.Itoa(motion.Rectangle.Y) + "-" +
						strconv.Itoa(motion.Rectangle.Width) + "-" + strconv.Itoa(motion.Rectangle.Height)
				}

				// Get the number of changes from the motion detection.
				numberOfChanges := motion.NumberOfChanges

				s := strconv.FormatInt(displayTimeSeconds, 10) + "_" + // start timestamp in seconds
					strconv.Itoa(len(strconv.FormatInt(displayTimeMilliseconds, 10))) + "-" + // length of milliseconds
					strconv.FormatInt(displayTimeMilliseconds, 10) + "_" + // milliseconds
					config.Name + "_" + // device name
					motionRectangleString + "_" + // region coordinates, we will not use this for continuous recording
					strconv.Itoa(numberOfChanges) + "_" + // number of changes
					"0" // + "_" + // duration of recording in milliseconds
					//utils.VERSION // version of the agent

				name := s + ".mp4"
				fullName := configDirectory + "/data/recordings/" + name

				// Running...
				log.Log.Info("capture.main.HandleRecordStream(motiondetection): recording started (" + name + ")" + " at " + strconv.FormatInt(displayTimeSeconds, 10) + " unix")

				// Get width and height from the camera.
				width := configuration.Config.Capture.IPCamera.Width
				height := configuration.Config.Capture.IPCamera.Height

				// Get SPS and PPS NALUs from the camera.
				spsNALUS := configuration.Config.Capture.IPCamera.SPSNALUs
				ppsNALUS := configuration.Config.Capture.IPCamera.PPSNALUs
				vpsNALUS := configuration.Config.Capture.IPCamera.VPSNALUs

				if len(spsNALUS) == 0 || len(ppsNALUS) == 0 {
					log.Log.Warning("capture.main.HandleRecordStream(motiondetection): missing SPS/PPS at recording start")
				}
				// Create a video file, and set the dimensions.
				mp4Video := video.NewMP4(fullName, spsNALUS, ppsNALUS, vpsNALUS, configuration.Config.Capture.MaxLengthRecording)
				mp4Video.SetWidth(width)
				mp4Video.SetHeight(height)

				if videoCodec == "H264" {
					videoTrack = mp4Video.AddVideoTrack("H264")
				} else if videoCodec == "H265" {
					videoTrack = mp4Video.AddVideoTrack("H265")
				}
				if audioCodec == "AAC" {
					audioTrack = mp4Video.AddAudioTrack("AAC")
				} else if audioCodec == "PCM_MULAW" {
					log.Log.Debug("capture.main.HandleRecordStream(continuous): no AAC audio codec detected, skipping audio track.")
				}

				for cursorError == nil {

					nextPkt, cursorError = recordingCursor.ReadPacket()
					if cursorError != nil {
						log.Log.Error("capture.main.HandleRecordStream(motiondetection): " + cursorError.Error())
					}

					now = time.Now().UnixMilli()
					select {
					case motion := <-communication.HandleMotion:
						motionTimestamp = now
						log.Log.Info("capture.main.HandleRecordStream(motiondetection): motion detected while recording. Expanding recording.")
						numberOfChanges := motion.NumberOfChanges
						log.Log.Info("capture.main.HandleRecordStream(motiondetection): Received message with recording data, detected changes to save: " + strconv.Itoa(numberOfChanges))
					default:
					}

					if (motionTimestamp+postRecording-now < 0 || now-startRecording > maxRecordingPeriod-500) && nextPkt.IsKeyFrame {
						log.Log.Info("capture.main.HandleRecordStream(motiondetection): timestamp+postRecording-now < 0  - " + strconv.FormatInt(motionTimestamp+postRecording-now, 10) + " < 0")
						log.Log.Info("capture.main.HandleRecordStream(motiondetection): now-startRecording > maxRecordingPeriod-500 - " + strconv.FormatInt(now-startRecording, 10) + " > " + strconv.FormatInt(maxRecordingPeriod-500, 10))
						log.Log.Info("capture.main.HandleRecordStream(motiondetection): closing recording (timestamp: " + strconv.FormatInt(motionTimestamp, 10) + ", postRecording: " + strconv.FormatInt(postRecording, 10) + ", now: " + strconv.FormatInt(now, 10) + ", startRecording: " + strconv.FormatInt(startRecording, 10) + ", maxRecordingPeriod: " + strconv.FormatInt(maxRecordingPeriod, 10))
						break
					}
					if pkt.IsKeyFrame && !start && pkt.CurrentTime >= startRecording {
						// We start the recording if we have a keyframe and the last duration is 0 or less than the current packet time.
						// It could be start we start from the beginning of the recording.
						log.Log.Debug("capture.main.HandleRecordStream(motiondetection): write frames")
						start = true
					}
					if start {
						pts := convertPTS(pkt.TimeLegacy)
						if pkt.IsVideo {
							log.Log.Debug("capture.main.HandleRecordStream(motiondetection): add video sample")
							if err := mp4Video.AddSampleToTrack(videoTrack, pkt.IsKeyFrame, pkt.Data, pts); err != nil {
								log.Log.Error("capture.main.HandleRecordStream(motiondetection): " + err.Error())
							}
						} else if pkt.IsAudio {
							log.Log.Debug("capture.main.HandleRecordStream(motiondetection): add audio sample")
							if pkt.Codec == "AAC" {
								if err := mp4Video.AddSampleToTrack(audioTrack, pkt.IsKeyFrame, pkt.Data, pts); err != nil {
									log.Log.Error("capture.main.HandleRecordStream(motiondetection): " + err.Error())
								}
							} else if pkt.Codec == "PCM_MULAW" {
								// TODO: transcode to AAC, some work to do..
								// We might need to use ffmpeg to transcode the audio to AAC.
								// For now we will skip the audio track.
								log.Log.Debug("capture.main.HandleRecordStream(motiondetection): no AAC audio codec detected, skipping audio track.")
							}
						}
					}

					pkt = nextPkt
				}

				// Update the last duration and last recording time.
				// This is used to determine if we need to start a new recording.
				lastRecordingTime = pkt.CurrentTime

				// This will close the recording and write the last packet.
				if len(mp4Video.SPSNALUs) == 0 && len(configuration.Config.Capture.IPCamera.SPSNALUs) > 0 {
					mp4Video.SPSNALUs = configuration.Config.Capture.IPCamera.SPSNALUs
				}
				if len(mp4Video.PPSNALUs) == 0 && len(configuration.Config.Capture.IPCamera.PPSNALUs) > 0 {
					mp4Video.PPSNALUs = configuration.Config.Capture.IPCamera.PPSNALUs
				}
				if len(mp4Video.VPSNALUs) == 0 && len(configuration.Config.Capture.IPCamera.VPSNALUs) > 0 {
					mp4Video.VPSNALUs = configuration.Config.Capture.IPCamera.VPSNALUs
				}
				if (videoCodec == "H264" && (len(mp4Video.SPSNALUs) == 0 || len(mp4Video.PPSNALUs) == 0)) ||
					(videoCodec == "H265" && (len(mp4Video.VPSNALUs) == 0 || len(mp4Video.SPSNALUs) == 0 || len(mp4Video.PPSNALUs) == 0)) {
					log.Log.Warning("capture.main.HandleRecordStream(motiondetection): closing MP4 without full parameter sets, moov may be incomplete")
				}
				mp4Video.Close(&config)
				log.Log.Info("capture.main.HandleRecordStream(motiondetection): file save: " + name)

				// Update the name of the recording with the duration.
				// We will update the name of the recording with the duration in milliseconds.
				if mp4Video.VideoTotalDuration > 0 {
					duration := mp4Video.VideoTotalDuration

					// Update the name with the duration in milliseconds.
					s := strconv.FormatInt(displayTimeSeconds, 10) + "_" +
						strconv.Itoa(len(strconv.FormatInt(displayTimeMilliseconds, 10))) + "-" +
						strconv.FormatInt(displayTimeMilliseconds, 10) + "_" +
						config.Name + "_" +
						motionRectangleString + "_" +
						strconv.Itoa(numberOfChanges) + "_" + // number of changes
						strconv.FormatInt(int64(duration), 10) // + "_" + // duration of recording in milliseconds
						//utils.VERSION // version of the agent

					oldName := name
					name = s + ".mp4"
					fullName = configDirectory + "/data/recordings/" + name
					log.Log.Info("capture.main.HandleRecordStream(motiondetection): renamed file from: " + oldName + " to: " + name)

					// Rename the file to the new name.
					err := os.Rename(
						configDirectory+"/data/recordings/"+oldName,
						configDirectory+"/data/recordings/"+s+".mp4")

					if err != nil {
						log.Log.Error("capture.main.HandleRecordStream(motiondetection): error renaming file: " + err.Error())
					}
				} else {
					log.Log.Info("capture.main.HandleRecordStream(motiondetection): no video data recorded, not renaming file.")
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

	// Start OpenTelemetry tracing
	ctxVerifyCamera, span := tracer.Start(context.Background(), "VerifyCamera", trace.WithSpanKind(trace.SpanKindServer))
	defer span.End()

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

		err := rtspClient.Connect(ctx, ctxVerifyCamera)
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

			err := rtspClient.Close(ctxVerifyCamera)
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

func Base64Image(captureDevice *Capture, communication *models.Communication, configuration *models.Configuration) string {
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
					imageResized, _ := utils.ResizeImage(&img, uint(configuration.Config.Capture.IPCamera.BaseWidth), uint(configuration.Config.Capture.IPCamera.BaseHeight))
					bytes, _ := utils.ImageToBytes(imageResized)
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

/*func convertPTS2(v int64) uint64 {
	return uint64(v) / 100
}*/
