package computervision

import (
	"image"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	geo "github.com/kellydunn/golang-geo"
	"github.com/kerberos-io/agent/machinery/src/capture"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/packets"
)

func ProcessMotion(motionCursor *packets.QueueCursor, configuration *models.Configuration, communication *models.Communication, mqttClient mqtt.Client, rtspClient capture.RTSPClient) {

	log.Log.Debug("ProcessMotion: started")
	config := configuration.Config

	var isPixelChangeThresholdReached = false
	var changesToReturn = 0

	pixelThreshold := config.Capture.PixelChangeThreshold
	// Might not be set in the config file, so set it to 150
	if pixelThreshold == 0 {
		pixelThreshold = 150
	}

	if config.Capture.Continuous == "true" {

		log.Log.Info("ProcessMotion: Continuous recording, so no motion detection.")

	} else {

		log.Log.Info("ProcessMotion: Motion detection enabled.")

		hubKey := config.HubKey
		deviceKey := config.Key

		// Initialise first 2 elements
		var imageArray [3]*image.Gray

		j := 0

		var cursorError error
		var pkt packets.Packet

		for cursorError == nil {
			pkt, cursorError = motionCursor.ReadPacket()
			// Check If valid package.
			if len(pkt.Data) > 0 && pkt.IsKeyFrame {
				grayImage, err := rtspClient.DecodePacketRaw(pkt)
				if err == nil {
					imageArray[j] = &grayImage
					j++
				}
			}
			if j == 3 {
				break
			}
		}

		// Calculate mask
		var polyObjects []geo.Polygon

		if config.Region != nil {
			for _, polygon := range config.Region.Polygon {
				coords := polygon.Coordinates
				poly := geo.Polygon{}
				for _, c := range coords {
					x := c.X
					y := c.Y
					p := geo.NewPoint(x, y)
					if !poly.Contains(p) {
						poly.Add(p)
					}
				}
				polyObjects = append(polyObjects, poly)
			}
		}

		img := imageArray[0]
		var coordinatesToCheck []int
		if img != nil {
			bounds := img.Bounds()
			rows := bounds.Dy()
			cols := bounds.Dx()

			// Make fixed size array of uinty8
			for y := 0; y < rows; y++ {
				for x := 0; x < cols; x++ {
					for _, poly := range polyObjects {
						point := geo.NewPoint(float64(x), float64(y))
						if poly.Contains(point) {
							coordinatesToCheck = append(coordinatesToCheck, y*cols+x)
						}
					}
				}
			}
		}

		// If no region is set, we'll skip the motion detection
		if len(coordinatesToCheck) > 0 {

			// Start the motion detection
			i := 0
			loc, _ := time.LoadLocation(config.Timezone)

			for cursorError == nil {
				pkt, cursorError = motionCursor.ReadPacket()

				// Check If valid package.
				if len(pkt.Data) == 0 || !pkt.IsKeyFrame {
					continue
				}

				grayImage, err := rtspClient.DecodePacketRaw(pkt)
				if err == nil {
					imageArray[2] = &grayImage
				}

				// Check if within time interval
				detectMotion := true
				timeEnabled := config.Time
				if timeEnabled != "false" {
					now := time.Now().In(loc)
					weekday := now.Weekday()
					hour := now.Hour()
					minute := now.Minute()
					second := now.Second()
					if config.Timetable != nil && len(config.Timetable) > 0 {
						timeInterval := config.Timetable[int(weekday)]
						if timeInterval != nil {
							start1 := timeInterval.Start1
							end1 := timeInterval.End1
							start2 := timeInterval.Start2
							end2 := timeInterval.End2
							currentTimeInSeconds := hour*60*60 + minute*60 + second
							if (currentTimeInSeconds >= start1 && currentTimeInSeconds <= end1) ||
								(currentTimeInSeconds >= start2 && currentTimeInSeconds <= end2) {

							} else {
								detectMotion = false
								log.Log.Info("ProcessMotion: Time interval not valid, disabling motion detection.")
							}
						}
					}
				}

				if config.Capture.Motion != "false" {

					// Remember additional information about the result of findmotion
					isPixelChangeThresholdReached, changesToReturn = FindMotion(imageArray, coordinatesToCheck, pixelThreshold)
					if detectMotion && isPixelChangeThresholdReached {

						// If offline mode is disabled, send a message to the hub
						if config.Offline != "true" {
							if mqttClient != nil {
								if hubKey != "" {
									message := models.Message{
										Payload: models.Payload{
											Action:   "motion",
											DeviceId: configuration.Config.Key,
											Value: map[string]interface{}{
												"timestamp": time.Now().Unix(),
											},
										},
									}
									payload, err := models.PackageMQTTMessage(configuration, message)
									if err == nil {
										mqttClient.Publish("kerberos/hub/"+hubKey, 0, false, payload)
									} else {
										log.Log.Info("ProcessMotion: failed to package MQTT message: " + err.Error())
									}
								} else {
									mqttClient.Publish("kerberos/agent/"+deviceKey, 2, false, "motion")
								}
							}
						}

						if config.Capture.Recording != "false" {
							dataToPass := models.MotionDataPartial{
								Timestamp:       time.Now().Unix(),
								NumberOfChanges: changesToReturn,
							}
							communication.HandleMotion <- dataToPass //Save data to the channel
						}
					}

					imageArray[0] = imageArray[1]
					imageArray[1] = imageArray[2]
					i++
				}
			}

			if img != nil {
				img = nil
			}
		}
	}

	log.Log.Debug("ProcessMotion: finished")
}

func FindMotion(imageArray [3]*image.Gray, coordinatesToCheck []int, pixelChangeThreshold int) (thresholdReached bool, changesDetected int) {
	image1 := imageArray[0]
	image2 := imageArray[1]
	image3 := imageArray[2]
	threshold := 60
	changes := AbsDiffBitwiseAndThreshold(image1, image2, image3, threshold, coordinatesToCheck)
	return changes > pixelChangeThreshold, changes
}

func AbsDiffBitwiseAndThreshold(img1 *image.Gray, img2 *image.Gray, img3 *image.Gray, threshold int, coordinatesToCheck []int) int {
	changes := 0
	for i := 0; i < len(coordinatesToCheck); i++ {
		pixel := coordinatesToCheck[i]
		diff := int(img3.Pix[pixel]) - int(img1.Pix[pixel])
		diff2 := int(img3.Pix[pixel]) - int(img2.Pix[pixel])
		if (diff > threshold || diff < -threshold) && (diff2 > threshold || diff2 < -threshold) {
			changes++
		}
	}
	return changes
}
