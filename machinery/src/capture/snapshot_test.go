package capture

import (
	"context"
	"testing"
	"time"

	"github.com/kerberos-io/agent/machinery/src/models"
	"github.com/kerberos-io/agent/machinery/src/packets"
)

func TestSnapshotReturnsAfterQueueClosure(t *testing.T) {
	for _, stream := range []string{"main", "sub"} {
		for _, format := range []string{"jpeg", "base64"} {
			for _, alreadyClosed := range []bool{true, false} {
				name := stream + "/" + format + "/closed_during_read"
				if alreadyClosed {
					name = stream + "/" + format + "/already_closed"
				}
				t.Run(name, func(t *testing.T) {
					communication := &models.Communication{}
					run := models.NewAgentRun(context.Background(), communication, false)
					queue := packets.NewQueue()
					captureDevice := &Capture{}
					if stream == "main" {
						captureDevice.SetMainClient("rtsp://unused.invalid/main")
						run.SetMainQueue(queue)
					} else {
						captureDevice.SetSubClient("rtsp://unused.invalid/sub")
						run.SetSubQueue(queue)
					}
					if err := run.Activate(); err != nil {
						t.Fatal(err)
					}
					defer queue.Close()
					if alreadyClosed {
						queue.Close()
					}
					done := make(chan bool, 1)
					go func() {
						if format == "jpeg" {
							image := JpegImage(captureDevice, communication)
							done <- image.Rect.Empty()
						} else {
							done <- Base64Image(captureDevice, communication, &models.Configuration{}) == ""
						}
					}()
					if !alreadyClosed {
						select {
						case <-done:
							t.Fatal("snapshot returned before a frame or queue closure")
						case <-time.After(10 * time.Millisecond):
						}
						queue.Close()
					}
					select {
					case empty := <-done:
						if !empty {
							t.Fatal("closed empty queue produced a snapshot")
						}
					case <-time.After(time.Second):
						t.Fatal("snapshot did not return after its packet queue closed")
					}
				})
			}
		}
	}
}
