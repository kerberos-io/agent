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
	var motionRectangles []models.MotionRectangle

	pixelThreshold := config.Capture.PixelChangeThreshold
	// Might not be set in the config file, so set it to 150
	if pixelThreshold == 0 {
		pixelThreshold = 150
	}

	// In motion mode we always run detection. In CONTINUOUS mode recording is
	// 24/7 so motion detection is normally skipped, BUT if a motion region is
	// configured we still run it so the live view can visualise the motion boxes
	// + region. In that case we only emit the motion EVENT — no motion-triggered
	// recording (continuous already records, and the recorder's motion branch
	// isn't draining HandleMotion in continuous mode).
	continuousMode := config.Capture.Continuous == "true"
	hasMotionRegion := config.Region != nil && len(config.Region.Polygon) > 0

	if continuousMode && !hasMotionRegion {

		log.Log.Info("computervision.main.ProcessMotion(): continuous recording enabled and no motion region configured, so no motion detection required.")

	} else {

		if continuousMode {
			log.Log.Info("computervision.main.ProcessMotion(): continuous recording enabled with a motion region, running motion detection for live-view visualisation only (no motion-triggered recording).")
		} else {
			log.Log.Info("computervision.main.ProcessMotion(): motion detected is enabled, so starting the motion detection.")
		}

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

		// A user might have set the base width and height for the IPCamera.
		// This means also the polygon coordinates are set to a specific width and height (which might be different than the actual packets
		// received from the IPCamera). So we will resize the polygon coordinates to the base width and height.
		baseWidthRatio := 1.0
		baseHeightRatio := 1.0
		baseWidth := config.Capture.IPCamera.BaseWidth
		baseHeight := config.Capture.IPCamera.BaseHeight
		if baseWidth > 0 && baseHeight > 0 {
			// We'll get the first image to calculate the ratio
			img := imageArray[0]
			if img != nil {
				bounds := img.Bounds()
				rows := bounds.Dy()
				cols := bounds.Dx()
				baseWidthRatio = float64(cols) / float64(baseWidth)
				baseHeightRatio = float64(rows) / float64(baseHeight)
			}
		}

		// Calculate mask
		var polyObjects []geo.Polygon
		if config.Region != nil {
			for _, polygon := range config.Region.Polygon {
				coords := polygon.Coordinates
				poly := geo.Polygon{}
				for _, c := range coords {
					x := c.X * baseWidthRatio
					y := c.Y * baseHeightRatio
					p := geo.NewPoint(x, y)
					if !poly.Contains(p) {
						poly.Add(p)
					}
				}
				polyObjects = append(polyObjects, poly)
			}
		}

		// Frame dimensions + the motion region polygon(s) in image space, shipped
		// with each motion event so the live view can draw a motion-debug overlay
		// (the boxes below + the detection region).
		var imageCols, imageRows int
		var regionPolygons [][]map[string]int
		if config.Region != nil {
			for _, polygon := range config.Region.Polygon {
				var pts []map[string]int
				for _, c := range polygon.Coordinates {
					pts = append(pts, map[string]int{
						"x": int(c.X * baseWidthRatio),
						"y": int(c.Y * baseHeightRatio),
					})
				}
				if len(pts) > 0 {
					regionPolygons = append(regionPolygons, pts)
				}
			}
		}

		img := imageArray[0]
		var coordinatesToCheck []int
		if img != nil {
			bounds := img.Bounds()
			rows := bounds.Dy()
			cols := bounds.Dx()
			imageCols = cols
			imageRows = rows

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

				// Run detection when motion is enabled, OR when we're in continuous
				// mode with a region: there config.Capture.Motion (the motion-RECORDING
				// switch) is irrelevant, so the configured region alone is enough to
				// emit motion events for the live-view overlay.
				if config.Capture.Motion != "false" || continuousMode {

					if detectMotion {

						// Remember additional information about the result of findmotion
						isPixelChangeThresholdReached, changesToReturn, motionRectangle, motionRectangles = FindMotion(imageArray, coordinatesToCheck, pixelThreshold)
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
													// Live-view motion-debug overlay data. The boxes/region
													// are in the MOTION frame's pixel space (width/height =
													// the stream motion ran on, i.e. the sub stream when
													// set). mainWidth/mainHeight are the MAIN stream's
													// dimensions so the live view can extrapolate the
													// boxes/region onto the high-res main view it shows —
													// we know both, so no guessing from the <video> element.
													"width":      imageCols,
													"height":     imageRows,
													"mainWidth":  configuration.Config.Capture.IPCamera.Width,
													"mainHeight": configuration.Config.Capture.IPCamera.Height,
													"regions":    motionRectangles,
													"polygon":    regionPolygons,
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

							// Trigger motion-based recording — but NOT in continuous mode:
							// there the recorder runs the continuous branch and does not
							// drain HandleMotion, so a (blocking) send would hang the motion
							// loop. In continuous mode we only publish the motion event above
							// for the live-view overlay.
							if config.Capture.Recording != "false" && !continuousMode {
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

func FindMotion(imageArray [3]*image.Gray, coordinatesToCheck []int, pixelChangeThreshold int) (thresholdReached bool, changesDetected int, motionRectangle models.MotionRectangle, motionRectangles []models.MotionRectangle) {
	image1 := imageArray[0]
	image2 := imageArray[1]
	image3 := imageArray[2]
	threshold := 60
	changes, motionRectangle, motionRectangles := AbsDiffBitwiseAndThreshold(image1, image2, image3, threshold, coordinatesToCheck)
	return changes > pixelChangeThreshold, changes, motionRectangle, motionRectangles
}

func AbsDiffBitwiseAndThreshold(img1 *image.Gray, img2 *image.Gray, img3 *image.Gray, threshold int, coordinatesToCheck []int) (int, models.MotionRectangle, []models.MotionRectangle) {
	changes := 0
	cols := img1.Bounds().Dx()
	rows := img1.Bounds().Dy()
	var pixelList [][]int
	for i := 0; i < len(coordinatesToCheck); i++ {
		pixel := coordinatesToCheck[i]
		diff := int(img3.Pix[pixel]) - int(img1.Pix[pixel])
		diff2 := int(img3.Pix[pixel]) - int(img2.Pix[pixel])
		if (diff > threshold || diff < -threshold) && (diff2 > threshold || diff2 < -threshold) {
			changes++
			// Store the pixel coordinates where the change is detected
			pixelList = append(pixelList, []int{pixel % cols, pixel / cols})
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

	// Cluster the changed pixels into separate bounding boxes so the live view can
	// visualise WHERE motion happened (a single overall rectangle is useless when
	// two objects move in opposite corners). Cheap grid-based connected components.
	motionRectangles := clusterMotionRectangles(pixelList, cols, rows)

	return changes, motionRectangle, motionRectangles
}

// clusterMotionRectangles groups the changed-pixel coordinates into a handful of
// bounding boxes using connected-components on a coarse grid (8-connectivity).
// It is intentionally lightweight — it runs only when the motion threshold is
// reached and the boxes are meant for a debug overlay, not precise detection.
func clusterMotionRectangles(pixelList [][]int, cols, rows int) []models.MotionRectangle {
	if len(pixelList) == 0 || cols <= 0 || rows <= 0 {
		return nil
	}

	// ~40 cells across the longest side keeps the grid small (cheap to cluster)
	// while still separating distinct motion blobs.
	const gridDim = 40
	cellW := cols / gridDim
	if cellW < 1 {
		cellW = 1
	}
	cellH := rows / gridDim
	if cellH < 1 {
		cellH = 1
	}
	gCols := (cols + cellW - 1) / cellW
	gRows := (rows + cellH - 1) / cellH

	grid := make([]bool, gCols*gRows)
	for _, p := range pixelList {
		cx := p[0] / cellW
		cy := p[1] / cellH
		if cx >= 0 && cx < gCols && cy >= 0 && cy < gRows {
			grid[cy*gCols+cx] = true
		}
	}

	visited := make([]bool, gCols*gRows)
	var rectangles []models.MotionRectangle
	const maxBoxes = 12
	stack := make([][2]int, 0, 64)

	for cy := 0; cy < gRows; cy++ {
		for cx := 0; cx < gCols; cx++ {
			idx := cy*gCols + cx
			if !grid[idx] || visited[idx] {
				continue
			}

			// Flood-fill this component (8-connectivity) and track its extent.
			minX, minY, maxX, maxY := cx, cy, cx, cy
			cellCount := 0
			stack = stack[:0]
			stack = append(stack, [2]int{cx, cy})
			visited[idx] = true
			for len(stack) > 0 {
				cur := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				ccx, ccy := cur[0], cur[1]
				cellCount++
				if ccx < minX {
					minX = ccx
				}
				if ccy < minY {
					minY = ccy
				}
				if ccx > maxX {
					maxX = ccx
				}
				if ccy > maxY {
					maxY = ccy
				}
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						nx, ny := ccx+dx, ccy+dy
						if nx < 0 || ny < 0 || nx >= gCols || ny >= gRows {
							continue
						}
						nIdx := ny*gCols + nx
						if grid[nIdx] && !visited[nIdx] {
							visited[nIdx] = true
							stack = append(stack, [2]int{nx, ny})
						}
					}
				}
			}

			// Skip single-cell specks (sensor noise) unless it's the only motion.
			if cellCount < 2 && len(pixelList) > 4 {
				continue
			}

			x := minX * cellW
			y := minY * cellH
			w := (maxX - minX + 1) * cellW
			h := (maxY - minY + 1) * cellH
			if x+w > cols {
				w = cols - x
			}
			if y+h > rows {
				h = rows - y
			}
			rectangles = append(rectangles, models.MotionRectangle{X: x, Y: y, Width: w, Height: h})
			if len(rectangles) >= maxBoxes {
				return rectangles
			}
		}
	}

	return rectangles
}
