package computervision

import (
	"bufio"
	"bytes"
	"image"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/kerberos-io/agent/machinery/src/capture"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/joy4/av/pubsub"
	"github.com/whorfin/go-libjpeg/jpeg"

	geo "github.com/kellydunn/golang-geo"
	"github.com/kerberos-io/joy4/av"
	"github.com/kerberos-io/joy4/cgo/ffmpeg"
)

func ProcessMotion(motionCursor *pubsub.QueueCursor, configuration *models.Configuration, communication *models.Communication, mqttClient mqtt.Client, decoder *ffmpeg.VideoDecoder, decoderMutex *sync.Mutex) { //, wg *sync.WaitGroup) {
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

		key := config.HubKey

		// Initialise first 2 elements
		var imageArray [3]*image.Gray

		j := 0

		var cursorError error
		var pkt av.Packet

		for cursorError == nil {
			pkt, cursorError = motionCursor.ReadPacket()
			// Check If valid package.
			if len(pkt.Data) > 0 && pkt.IsKeyFrame {
				grayImage, err := GetGrayImage(pkt, decoder, decoderMutex)
				if err == nil {
					imageArray[j] = grayImage
					j++
				}
			}
			if j == 3 {
				break
			}
		}

		img := imageArray[0]
		if img != nil {

			// Calculate mask
			var polyObjects []geo.Polygon
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

			bounds := img.Bounds()
			rows := bounds.Dx()
			cols := bounds.Dy()

			// Make fixed size array of uinty8
			var coordinatesToCheck []int
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

			// Start the motion detection
			i := 0
			loc, _ := time.LoadLocation(config.Timezone)

			for cursorError == nil {
				pkt, cursorError = motionCursor.ReadPacket()

				// Check If valid package.
				if len(pkt.Data) == 0 || !pkt.IsKeyFrame {
					continue
				}

				grayImage, err := GetGrayImage(pkt, decoder, decoderMutex)
				if err == nil {
					imageArray[2] = grayImage
				}

				// Store snapshots (jpg) or hull.
				files, err := ioutil.ReadDir("./data/snapshots")
				if err == nil {
					rgbImage, err := GetRawImage(pkt, decoder, decoderMutex)
					if err == nil {
						sort.Slice(files, func(i, j int) bool {
							return files[i].ModTime().Before(files[j].ModTime())
						})
						if len(files) > 3 {
							os.Remove("./data/snapshots/" + files[0].Name())
						}

						// Save image
						t := strconv.FormatInt(time.Now().Unix(), 10)
						f, err := os.Create("./data/snapshots/" + t + ".jpg")
						if err == nil {
							jpeg.Encode(f, &rgbImage.Image, &jpeg.EncoderOptions{Quality: 30})
							f.Close()
						}
					}
					rgbImage.Free()
				}

				// Check if within time interval
				detectMotion := true
				now := time.Now().In(loc)
				weekday := now.Weekday()
				hour := now.Hour()
				minute := now.Minute()
				second := now.Second()
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
						log.Log.Debug("ProcessMotion: Time interval not valid, disabling motion detection.")
					}
				}

				// Remember additional information about the result of findmotion
				isPixelChangeThresholdReached, changesToReturn = FindMotion(imageArray, coordinatesToCheck, pixelThreshold)

				if detectMotion && isPixelChangeThresholdReached {

					if mqttClient != nil {
						mqttClient.Publish("kerberos/"+key+"/device/"+config.Key+"/motion", 2, false, "motion")
					}

					//FIXME: In the future MotionDataPartial should be replaced with MotionDataFull
					dataToPass := models.MotionDataPartial{
						Timestamp:       time.Now().Unix(),
						NumberOfChanges: changesToReturn,
					}
					communication.HandleMotion <- dataToPass //Save data to the channel
				}

				imageArray[0] = imageArray[1]
				imageArray[1] = imageArray[2]
				i++
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
	changes := AbsDiffBitwiseAndThreshold(image1, image2, image3, pixelChangeThreshold, coordinatesToCheck)
	return changes > pixelChangeThreshold, changes
}

func GetGrayImage(pkt av.Packet, dec *ffmpeg.VideoDecoder, decoderMutex *sync.Mutex) (*image.Gray, error) {
	img, err := capture.DecodeImage(pkt, dec, decoderMutex)

	// Do a deep copy of the image
	imgDeepCopy := image.NewGray(img.ImageGray.Bounds())
	imgDeepCopy.Stride = img.ImageGray.Stride
	copy(imgDeepCopy.Pix, img.ImageGray.Pix)

	// Cleanup of underlaying data
	img.Free()

	return imgDeepCopy, err
}

func GetRawImage(pkt av.Packet, dec *ffmpeg.VideoDecoder, decoderMutex *sync.Mutex) (*ffmpeg.VideoFrame, error) {
	img, err := capture.DecodeImage(pkt, dec, decoderMutex)
	// We'll need to free up ourselves ;) using -> img.Free()
	return img, err
}

func ImageToBytes(img image.Image) ([]byte, error) {
	buffer := new(bytes.Buffer)
	w := bufio.NewWriter(buffer)
	err := jpeg.Encode(w, img, &jpeg.EncoderOptions{Quality: 30})
	return buffer.Bytes(), err
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
