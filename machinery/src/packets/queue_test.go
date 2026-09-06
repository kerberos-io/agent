package packets

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type observedCancelContext struct {
	done        chan struct{}
	cancelled   atomic.Bool
	errCalls    atomic.Int32
	waiting     chan struct{}
	waitingOnce sync.Once
}

func newObservedCancelContext() *observedCancelContext {
	return &observedCancelContext{
		done:    make(chan struct{}),
		waiting: make(chan struct{}),
	}
}

func (c *observedCancelContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *observedCancelContext) Done() <-chan struct{}       { return c.done }
func (c *observedCancelContext) Value(any) any               { return nil }

func (c *observedCancelContext) Err() error {
	if c.errCalls.Add(1) >= 2 {
		c.waitingOnce.Do(func() { close(c.waiting) })
	}
	if c.cancelled.Load() {
		return context.Canceled
	}
	return nil
}

func (c *observedCancelContext) Cancel() {
	c.cancelled.Store(true)
	close(c.done)
}

func TestQueueCursorReadPacketContextCancelsWhileQueueIsIdle(t *testing.T) {
	queue := NewQueue()
	cursor := queue.Latest()
	ctx := newObservedCancelContext()
	done := make(chan error, 1)

	go func() {
		_, err := cursor.ReadPacketContext(ctx)
		done <- err
	}()

	select {
	case <-ctx.waiting:
	case <-time.After(time.Second):
		t.Fatal("ReadPacketContext() did not reach the queue wait")
	}
	ctx.Cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ReadPacketContext() error = %v, want %v", err, context.Canceled)
		}
	case <-time.After(time.Second):
		t.Fatal("ReadPacketContext() did not return after cancellation")
	}
}

func TestQueueCursorReadPacketDoesNotAllocate(t *testing.T) {
	queue := NewQueue()
	for i := 0; i < 200; i++ {
		if err := queue.WritePacket(Packet{Data: []byte{1}}); err != nil {
			t.Fatal(err)
		}
	}
	cursor := queue.Oldest()

	allocations := testing.AllocsPerRun(100, func() {
		if _, err := cursor.ReadPacket(); err != nil {
			t.Fatal(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("ReadPacket() allocations = %.2f, want 0", allocations)
	}
}
