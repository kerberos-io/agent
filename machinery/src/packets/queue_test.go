package packets

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestQueueCursorReadPacketContextCancelsWhileQueueIsIdle(t *testing.T) {
	queue := NewQueue()
	cursor := queue.Latest()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		_, err := cursor.ReadPacketContext(ctx)
		done <- err
	}()

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ReadPacketContext() error = %v, want %v", err, context.Canceled)
		}
	case <-time.After(time.Second):
		t.Fatal("ReadPacketContext() did not return after cancellation")
	}
}
