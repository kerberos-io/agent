package computervision

import (
	"bytes"
	"image"
	"image/jpeg"
	"io/ioutil"
	"math"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/disintegration/imaging"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/kerberos-io/agent/machinery/src/capture"
	"github.com/kerberos-io/agent/machinery/src/log"
	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/joy4/av/pubsub"

	geo "github.com/kellydunn/golang-geo"
	"github.com/kerberos-io/joy4/av"
	"github.com/kerberos-io/joy4/cgo/ffmpeg"
)

func ProcessMotion(motionCursor *pubsub.QueueCursor, configuration *models.Configuration, communication *models.Communication, mqttClient mqtt.Client, decoder *ffmpeg.VideoDecoder, decoderMutex *sync.Mutex) { //, wg *sync.WaitGroup) {
	log.Log.Debug("ProcessMotion: started")
	config := configuration.Config

	var isPixelChangeThresholdReached = false
	var changesToReturn = 0

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
				grayImage, err := GetImage(pkt, decoder, decoderMutex)
				if err == nil {
					imageArray[j] = grayImage
					j++
				}
			}
			if j == 2 {
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
			var coordinatesToCheck [][]int
			for y := 0; y < rows; y++ {
				for x := 0; x < cols; x++ {
					for _, poly := range polyObjects {
						point := geo.NewPoint(float64(x), float64(y))
						if poly.Contains(point) {
							coordinatesToCheck = append(coordinatesToCheck, []int{x, y})
							break
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

				//rgb := GetImage(pkt, decoder, decoderMutex)
				//gray, _ := ToGray(rgb)
				//matArray[2] = &gray
				grayImage, err := GetImage(pkt, decoder, decoderMutex)
				if err == nil {
					imageArray[2] = grayImage
				}

				// Store snapshots (jpg) or hull.
				files, err := ioutil.ReadDir("./data/snapshots")
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
						jpeg.Encode(f, grayImage, nil)
						f.Close()
					}
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
				isPixelChangeThresholdReached, changesToReturn = FindMotion(imageArray, coordinatesToCheck, config.Capture.PixelChangeThreshold)

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

func FindMotion(imageArray [3]*image.Gray, coordinatesToCheck [][]int, pixelChangeThreshold int) (thresholdReached bool, changesDetected int) {

	//image1 := imageArray[0]
	image2 := imageArray[1]
	image3 := imageArray[2]

	// Calculate the absolute difference between the first and second image.
	//diff1 := AbsDiff(image3, image1)
	diff2 := AbsDiff(image3, image2)

	// Calculate the bitwise AND between the first and second image.
	//and := BitwiseAnd(diff1, diff2)

	// Do threshold§
	threshold := Threshold(diff2, 30)

	// Erode and dilate the image to remove noise.
	erode := Erode(threshold, 3)

	changes := 0
	for _, c := range coordinatesToCheck {
		pixel := c[0] + c[1]*erode.Stride
		value := erode.Pix[pixel]
		if value > 0 {
			changes++
		}
	}

	return changes > pixelChangeThreshold, changes
}

func GetImage(pkt av.Packet, dec *ffmpeg.VideoDecoder, decoderMutex *sync.Mutex) (*image.Gray, error) {
	img, err := capture.DecodeImage(pkt, dec, decoderMutex)
	return &img.ImageGray, err
}

func ResizeImage(img image.Image, width int, height int) *image.NRGBA {
	scaledImage := imaging.Resize(img, width, height, imaging.NearestNeighbor)
	return scaledImage
}

func ResizeDownscaleImage(img image.Image, dxy int) *image.NRGBA {
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()
	dstImage128 := imaging.Resize(img, width/dxy, height/dxy, imaging.NearestNeighbor)
	return dstImage128
}

func ImageToBytes(img image.Image) ([]byte, error) {
	buffer := new(bytes.Buffer)
	err := jpeg.Encode(buffer, img, &jpeg.Options{Quality: 40})
	return buffer.Bytes(), err
}

func Threshold(img image.Gray, threshold int) (thresholded image.Gray) {
	thresholded = image.Gray{
		Pix:    make([]uint8, len(img.Pix)),
		Stride: img.Stride,
		Rect:   img.Rect,
	}
	for i := 0; i < len(img.Pix); i++ {
		if int(img.Pix[i]) > threshold {
			thresholded.Pix[i] = 255
		} else {
			thresholded.Pix[i] = 0
		}
	}
	return thresholded
}

func AbsDiff(img1 *image.Gray, img2 *image.Gray) (diff image.Gray) {
	diff = image.Gray{
		Pix:    make([]uint8, len(img1.Pix)),
		Stride: img1.Stride,
		Rect:   img1.Rect,
	}
	for i := 0; i < len(img1.Pix); i++ {
		diff.Pix[i] = uint8(math.Abs(float64(img1.Pix[i]) - float64(img2.Pix[i])))
	}
	return diff
}

func BitwiseAnd(img1 image.Gray, img2 image.Gray) (and image.Gray) {
	and = image.Gray{
		Pix:    make([]uint8, len(img1.Pix)),
		Stride: img1.Stride,
		Rect:   img1.Rect,
	}
	for i := 0; i < len(img1.Pix); i++ {
		and.Pix[i] = img1.Pix[i] & img2.Pix[i]
	}
	return and
}

func Erode(img image.Gray, kernelSize int) (eroded image.Gray) {
	eroded = image.Gray{
		Pix:    make([]uint8, len(img.Pix)),
		Stride: img.Stride,
		Rect:   img.Rect,
	}
	for i := 0; i < len(img.Pix); i++ {
		eroded.Pix[i] = img.Pix[i]
	}
	return eroded
}
