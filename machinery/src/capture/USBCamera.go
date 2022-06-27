package capture

import (
	"strconv"
	"time"

	"github.com/kerberos-io/agent/machinery/src/log"
	"gocv.io/x/gocv"
)

func TestUSBCamera(deviceID string) {
	webcam, err := gocv.OpenVideoCapture(deviceID)
	if err != nil {
		log.Log.Error("Error opening video capture device: " + deviceID)
		return
	}
	defer webcam.Close()
	buf := gocv.NewMat()
	defer buf.Close()

	ok := webcam.Read(&buf)

	if ok {

		now := time.Now().Unix()
		saveFile := "./data/capture-test/" + strconv.FormatInt(now, 10) + ".mp4"
		fps := 20.0 // webcam.Get(gocv.VideoCaptureFPS)
		writer, err := gocv.VideoWriterFile(saveFile, "X264", fps, buf.Cols(), buf.Rows(), true)
		if err != nil {
			log.Log.Error("error opening video writer device: " + saveFile)
			return
		}
		defer writer.Close()

		//window := gocv.NewWindow("Hello")
		log.Log.Info("Start reading device: " + deviceID)
		for i := 0; i < 100; i++ {
			if ok := webcam.Read(&buf); !ok {
				log.Log.Error("Device closed: " + deviceID)
				return
			}
			if buf.Empty() {
				continue
			}

			writer.Write(buf)
			//window.IMShow(buf)
			//window.WaitKey(1)

			log.Log.Info("Read frame")
		}

	}
	log.Log.Info("Done. Close videocapture and recording file.")
}
