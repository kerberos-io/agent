package computervision

import (
	"image"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	geo "github.com/kellydunn/golang-geo"
	"github.com/kerberos-io/agent/machinery/src/capture"
	"github.com/kerberos-io/agent/machinery/src/conditions"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/packets"
)

func ProcessMotion(motionCursor *packets.QueueCursor, configuration *models.Configuration, communication *models.Communication, mqttClient mqtt.Client, rtspClient capture.RTSPClient) {

	log.Log.Debug("computervision.main.ProcessMotion(): start motion detection")
	config := configuration.Config
	loc, _ := time.LoadLocation(config.Timezone)

	var isPixelChangeThresholdReached = false
	var changesToReturn = 0
	var motionRectangle models.MotionRectangle

	pixelThreshold := config.Capture.PixelChangeThreshold
	// Might not be set in the config file, so set it to 150
	if pixelThreshold == 0 {
		pixelThreshold = 150
	}

	if config.Capture.Continuous == "true" {

		log.Log.Info("computervision.main.ProcessMotion(): you've enabled continuous recording, so no motion detection required.")

	} else {

		log.Log.Info("computervision.main.ProcessMotion(): motion detected is enabled, so starting the motion detection.")

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

				// We might have different conditions enabled such as time window or uri response.
				// We'll validate those conditions and if not valid we'll not do anything.
				detectMotion, err := conditions.Validate(loc, configuration)
				if !detectMotion && err != nil {
					log.Log.Debug("computervision.main.ProcessMotion(): " + err.Error() + ".")
				}

				if config.Capture.Motion != "false" {

					if detectMotion {

						// Remember additional information about the result of findmotion
						isPixelChangeThresholdReached, changesToReturn, motionRectangle = FindMotion(imageArray, coordinatesToCheck, pixelThreshold)
						if isPixelChangeThresholdReached {

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
											mqttClient.Publish("kerberos/hub/"+hubKey, 2, false, payload)
										} else {
											log.Log.Info("computervision.main.ProcessMotion(): failed to package MQTT message: " + err.Error())
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
									Rectangle:       motionRectangle,
								}
								communication.HandleMotion <- dataToPass //Save data to the channel
							}
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

	log.Log.Debug("computervision.main.ProcessMotion(): stop the motion detection.")
}

func FindMotion(imageArray [3]*image.Gray, coordinatesToCheck []int, pixelChangeThreshold int) (thresholdReached bool, changesDetected int, motionRectangle models.MotionRectangle) {
	image1 := imageArray[0]
	image2 := imageArray[1]
	image3 := imageArray[2]
	threshold := 60
	changes, motionRectangle := AbsDiffBitwiseAndThreshold(image1, image2, image3, threshold, coordinatesToCheck)
	return changes > pixelChangeThreshold, changes, motionRectangle
}

func AbsDiffBitwiseAndThreshold(img1 *image.Gray, img2 *image.Gray, img3 *image.Gray, threshold int, coordinatesToCheck []int) (int, models.MotionRectangle) {
	changes := 0
	var pixelList [][]int
	for i := 0; i < len(coordinatesToCheck); i++ {
		pixel := coordinatesToCheck[i]
		diff := int(img3.Pix[pixel]) - int(img1.Pix[pixel])
		diff2 := int(img3.Pix[pixel]) - int(img2.Pix[pixel])
		if (diff > threshold || diff < -threshold) && (diff2 > threshold || diff2 < -threshold) {
			changes++
			// Store the pixel coordinates where the change is detected
			pixelList = append(pixelList, []int{pixel % img1.Bounds().Dx(), pixel / img1.Bounds().Dx()})
		}
	}

	// Calculate rectangle of pixelList (startX, startY, endX, endY)
	var motionRectangle models.MotionRectangle
	if len(pixelList) > 0 {
		startX := pixelList[0][0]
		startY := pixelList[0][1]
		endX := startX
		endY := startY
		for _, pixel := range pixelList {
			if pixel[0] < startX {
				startX = pixel[0]
			}
			if pixel[1] < startY {
				startY = pixel[1]
			}
			if pixel[0] > endX {
				endX = pixel[0]
			}
			if pixel[1] > endY {
				endY = pixel[1]
			}
		}
		log.Log.Debugf("Rectangle of changes detected: startX: %d, startY: %d, endX: %d, endY: %d", startX, startY, endX, endY)
		motionRectangle = models.MotionRectangle{
			X:      startX,
			Y:      startY,
			Width:  endX - startX,
			Height: endY - startY,
		}
		log.Log.Debugf("Motion rectangle: %+v", motionRectangle)
	}
	return changes, motionRectangle
}
