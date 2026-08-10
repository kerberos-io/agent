package mqtt

import (
	"testing"
	"time"

	"github.com/kerberos-io/agent/machinery/src/models"
)

func TestEnqueueLatestAudioReplacesOldestFrameWhenFull(t *testing.T) {
	audioChannel := make(chan models.AudioDataPartial, 2)
	audioChannel <- models.AudioDataPartial{Timestamp: 1}
	audioChannel <- models.AudioDataPartial{Timestamp: 2}

	dropped := enqueueLatestAudio(audioChannel, models.AudioDataPartial{Timestamp: 3})
	if !dropped {
		t.Fatal("enqueueLatestAudio() dropped = false, want true")
	}

	first := <-audioChannel
	second := <-audioChannel
	if first.Timestamp != 2 || second.Timestamp != 3 {
		t.Fatalf("queued timestamps = (%d, %d), want (2, 3)", first.Timestamp, second.Timestamp)
	}
}

func TestEnqueueLatestAudioDoesNotBlockNilChannel(t *testing.T) {
	done := make(chan struct{})
	go func() {
		enqueueLatestAudio(nil, models.AudioDataPartial{Timestamp: 1})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("enqueueLatestAudio() blocked on a nil channel")
	}
}
