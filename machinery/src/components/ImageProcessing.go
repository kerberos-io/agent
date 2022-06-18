package components

import (
	"image"
	"io/ioutil"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/joy4/av/pubsub"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	geo "github.com/kellydunn/golang-geo"
	"github.com/kerberos-io/joy4/av"
	"github.com/kerberos-io/joy4/cgo/ffmpeg"
	"gocv.io/x/gocv"
)

func DecodeImage(pkt av.Packet, decoder *ffmpeg.VideoDecoder, decoderMutex *sync.Mutex) (*ffmpeg.VideoFrame, error) {
	decoderMutex.Lock()
	img, err := decoder.Decode(pkt.Data)
	decoderMutex.Unlock()
	return img, err
}

func GetRGBImage(pkt av.Packet, dec *ffmpeg.VideoDecoder, decoderMutex *sync.Mutex) gocv.Mat {
	var rgb gocv.Mat
	img, err := DecodeImage(pkt, dec, decoderMutex)
	if err == nil && img != nil {
		rgb, _ = ToRGB8(img.Image)
		gocv.Resize(rgb, &rgb, image.Pt(rgb.Cols()/4, rgb.Rows()/4), 0, 0, gocv.InterpolationArea)
	}
	return rgb
}

func GetImage(pkt av.Packet, dec *ffmpeg.VideoDecoder, decoderMutex *sync.Mutex) gocv.Mat {
	var gray gocv.Mat
	img, err := DecodeImage(pkt, dec, decoderMutex)

	// Check if we need to scale down.
	width := img.Width()
	height := img.Height()
	newWidth := width
	newHeight := height

	// Try minify twice.
	scaleFactor := 1.0
	if newWidth > 800 {
		newWidth = width / 2
		newHeight = height / 2
		scaleFactor *= 2
	}
	if newWidth > 800 {
		newWidth = width / 2
		newHeight = height / 2
		scaleFactor *= 2
	}
	if newWidth > 800 {
		newWidth = width / 2
		newHeight = height / 2
		scaleFactor *= 2
	}

	if err == nil && img != nil {
		im := img.Image
		rgb, _ := ToRGB8(im)
		img.Free()
		if scaleFactor > 1 {
			gocv.Resize(rgb, &rgb, image.Pt(newWidth, newHeight), 0, 0, gocv.InterpolationArea)
		}
		gray = gocv.NewMat()
		gocv.CvtColor(rgb, &gray, gocv.ColorBGRToGray)
		rgb.Close()
	}
	return gray
}

func ToRGB8(img image.YCbCr) (gocv.Mat, error) {
	bounds := img.Bounds()
	x := bounds.Dx()
	y := bounds.Dy()
	bytes := make([]byte, 0, x*y*3)
	for j := bounds.Min.Y; j < bounds.Max.Y; j++ {
		for i := bounds.Min.X; i < bounds.Max.X; i++ {
			r, g, b, _ := img.At(i, j).RGBA()
			bytes = append(bytes, byte(b>>8), byte(g>>8), byte(r>>8))
		}
	}
	return gocv.NewMatFromBytes(y, x, gocv.MatTypeCV8UC3, bytes)
}

func ProcessMotion(log Logging, motionCursor *pubsub.QueueCursor, config models.Config, name string, mqc mqtt.Client, packets <-chan av.Packet, motion chan<- int64, decoder *ffmpeg.VideoDecoder, decoderMutex *sync.Mutex) {

	if config.Capture.Continuous == "true" {

		log.Info("Disabled Detecting motion...")

	} else {

		log.Info("Start Detecting motion...")

		key := ""
		if config.Cloud == "s3" && config.S3.Publickey != "" {
			key = config.S3.Publickey
		} else if config.Cloud == "kstorage" && config.KStorage.CloudKey != "" {
			key = config.KStorage.CloudKey
		}

		// Initialise first 2 elements
		var matArray [3]*gocv.Mat
		j := 0

		//for pkt := range packets {
		var cursorError error
		var pkt av.Packet

		for cursorError == nil {
			pkt, cursorError = motionCursor.ReadPacket()
			// Check If valid package.
			if len(pkt.Data) > 0 && pkt.IsKeyFrame {
				rgb := GetImage(pkt, decoder, decoderMutex)
				matArray[j] = &rgb
				j++
			}
			if j == 2 {
				break
			}
		}

		img := matArray[0]
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

			rows := img.Rows()
			cols := img.Cols()
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

				rgb := GetImage(pkt, decoder, decoderMutex)
				matArray[2] = &rgb

				// Store snapshots (jpg) or hull.
				if i%3 == 0 {
					files, err := ioutil.ReadDir("./data/snapshots")
					if err == nil {
						sort.Slice(files, func(i, j int) bool {
							return files[i].ModTime().Before(files[j].ModTime())
						})
						if len(files) > 3 {
							os.Remove("./data/snapshots/" + files[0].Name())
						}
					}
					t := strconv.FormatInt(time.Now().Unix(), 10)
					gocv.IMWrite("./data/snapshots/"+t+".png", rgb)
				}

				// Check if continuous recording.
				if config.Capture.Continuous == "true" {

					// Do not do anything! Just sleep as there is no
					// motion detection needed

				} else { // Do motion detection.

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
							log.Debug("Disabled: not within time interval.")
						}
					}

					if detectMotion && FindMotion(matArray, coordinatesToCheck, log) {
						// TODO create object for motion
						mqc.Publish("kerberos/"+key+"/device/"+config.Key+"/motion", 2, false, "motion")
						motion <- time.Now().Unix()
					}
				}

				matArray[0].Close()
				matArray[0] = matArray[1]
				matArray[1] = matArray[2]
				i++
				runtime.GC()
				debug.FreeOSMemory()
			}
		}
		if img != nil {
			img.Close()
		}
		runtime.GC()
		debug.FreeOSMemory()
		log.Info("Stopped motion")
	}
}

func FindMotion(matArray [3]*gocv.Mat, coordinatesToCheck [][]int, log Logging) bool {

	h1 := gocv.NewMat()
	gocv.AbsDiff(*matArray[2], *matArray[0], &h1)
	h2 := gocv.NewMat()
	gocv.AbsDiff(*matArray[2], *matArray[1], &h2)

	and := gocv.NewMat()
	gocv.BitwiseAnd(h1, h2, &and)
	h1.Close()
	h2.Close()

	thresh := gocv.NewMat()
	gocv.Threshold(and, &thresh, 30.0, 255.0, gocv.ThresholdBinary)
	and.Close()

	kernel := gocv.GetStructuringElement(gocv.MorphRect, image.Pt(3, 3))
	eroded := gocv.NewMat()
	gocv.Erode(thresh, &eroded, kernel)
	thresh.Close()
	kernel.Close()

	changes := 0
	for _, c := range coordinatesToCheck {
		value := eroded.GetUCharAt(c[1], c[0])
		if value > 0 {
			changes++
		}
	}
	//gocv.IMWrite("./data/debug/"+ strconv.Itoa(changes)  +".png", eroded)

	eroded.Close()

	log.Info("Number of changes detected:" + strconv.Itoa(changes))

	if changes > 75 {
		return true
	}
	return false
}
