package models

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kerberos-io/agent/machinery/src/lifecycle"
	"github.com/kerberos-io/agent/machinery/src/packets"
)

type fakeAgentRunClient struct {
	mu       sync.Mutex
	name     string
	order    *[]string
	calls    int
	closeErr error
}

func (c *fakeAgentRunClient) Close(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	*c.order = append(*c.order, c.name)
	return c.closeErr
}

func (c *fakeAgentRunClient) Calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func TestAgentRunOwnsAndShutsDownResources(t *testing.T) {
	communication := &Communication{
		HandleStream: make(chan string, 1),
		HandleUpload: make(chan string, 1),
	}
	run := NewAgentRun(context.Background(), communication, true)
	mainQueue := packets.NewQueue()
	subQueue := packets.NewQueue()
	run.SetQueues(mainQueue, subQueue)

	var order []string
	mainClient := &fakeAgentRunClient{name: "main", order: &order}
	subClient := &fakeAgentRunClient{name: "sub", order: &order}
	backchannelClient := &fakeAgentRunClient{name: "backchannel", order: &order}
	run.SetMainClient(mainClient)
	run.SetSubClient(subClient)
	run.SetBackchannelClient(backchannelClient)
	run.SetClientRelease(func() {
		order = append(order, "release")
	})

	taskStarted := make(chan struct{})
	if err := run.Go("worker", lifecycle.TaskPolicy{}, func(ctx context.Context) error {
		close(taskStarted)
		<-ctx.Done()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := run.Activate(); err != nil {
		t.Fatal(err)
	}
	run.Seal()
	<-taskStarted

	report := run.Shutdown(context.Background(), errors.New("test shutdown"))
	if !report.Complete {
		t.Fatalf("shutdown report = %+v, want complete", report)
	}
	if communication.CurrentRun() != nil {
		t.Fatal("shutdown left the run attached")
	}
	if got, want := order, []string{"main", "sub", "backchannel", "release"}; !equalStrings(got, want) {
		t.Fatalf("resource order = %v, want %v", got, want)
	}
	if !report.UploadStopDelivered || !report.StreamStopDelivered {
		t.Fatalf("stop delivery = upload:%t stream:%t, want both", report.UploadStopDelivered, report.StreamStopDelivered)
	}
	select {
	case value := <-communication.HandleUpload:
		if value != "stop" {
			t.Fatalf("upload stop = %q, want stop", value)
		}
	default:
		t.Fatal("upload stop was not delivered")
	}
	select {
	case value := <-communication.HandleStream:
		if value != "stop" {
			t.Fatalf("stream stop = %q, want stop", value)
		}
	default:
		t.Fatal("stream stop was not delivered")
	}
	if _, err := mainQueue.Latest().ReadPacket(); err == nil {
		t.Fatal("main queue remained open")
	}
	if _, err := subQueue.Latest().ReadPacket(); err == nil {
		t.Fatal("sub queue remained open")
	}
	if _, ok := <-run.LiveHDHandshakes(); ok {
		t.Fatal("handshake channel remained open")
	}
	if _, ok := <-run.MotionEvents(); ok {
		t.Fatal("motion channel remained open")
	}
	if _, ok := <-run.ONVIFActions(); ok {
		t.Fatal("ONVIF channel remained open")
	}
}

func TestAgentRunShutdownIsConcurrentAndIdempotent(t *testing.T) {
	communication := &Communication{}
	run := NewAgentRun(context.Background(), communication, false)
	var order []string
	client := &fakeAgentRunClient{name: "main", order: &order}
	run.SetMainClient(client)
	if err := run.Activate(); err != nil {
		t.Fatal(err)
	}
	run.Seal()

	var callers sync.WaitGroup
	reports := make(chan AgentRunShutdownReport, 20)
	for index := 0; index < 20; index++ {
		callers.Add(1)
		go func() {
			defer callers.Done()
			reports <- run.Shutdown(context.Background(), errors.New("test shutdown"))
		}()
	}
	callers.Wait()
	close(reports)

	for report := range reports {
		if !report.Complete {
			t.Fatalf("shutdown report = %+v, want complete", report)
		}
	}
	if got := client.Calls(); got != 1 {
		t.Fatalf("client Close() calls = %d, want 1", got)
	}
}

func TestAgentRunStaleShutdownDoesNotDetachNewRun(t *testing.T) {
	communication := &Communication{}
	oldRun := NewAgentRun(context.Background(), communication, false)
	if err := oldRun.Activate(); err != nil {
		t.Fatal(err)
	}
	if !communication.detachRun(oldRun) {
		t.Fatal("failed to detach old run during test setup")
	}

	newRun := NewAgentRun(context.Background(), communication, false)
	if err := newRun.Activate(); err != nil {
		t.Fatal(err)
	}
	newRun.Seal()
	t.Cleanup(func() {
		newRun.Shutdown(context.Background(), errors.New("test complete"))
	})

	oldRun.Shutdown(context.Background(), errors.New("stale shutdown"))
	if got := communication.CurrentRun(); got != newRun {
		t.Fatalf("current run = %p, want new run %p", got, newRun)
	}
}

func TestAgentRunRejectsOverlap(t *testing.T) {
	communication := &Communication{}
	first := NewAgentRun(context.Background(), communication, false)
	if err := first.Activate(); err != nil {
		t.Fatal(err)
	}
	first.Seal()
	t.Cleanup(func() {
		first.Shutdown(context.Background(), errors.New("test complete"))
	})

	second := NewAgentRun(context.Background(), communication, false)
	if err := second.Activate(); !errors.Is(err, ErrAgentRunActive) {
		t.Fatalf("Activate() error = %v, want ErrAgentRunActive", err)
	}
	second.Shutdown(context.Background(), errors.New("test complete"))
}

func TestAgentRunRetainsOwnershipUntilWorkersStop(t *testing.T) {
	communication := &Communication{}
	run := NewAgentRun(context.Background(), communication, false)
	release := make(chan struct{})
	started := make(chan struct{})
	if err := run.Go("blocked", lifecycle.TaskPolicy{}, func(context.Context) error {
		close(started)
		<-release
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := run.Activate(); err != nil {
		t.Fatal(err)
	}
	run.Seal()
	<-started

	shutdownDone := make(chan AgentRunShutdownReport, 1)
	go func() {
		shutdownDone <- run.Shutdown(context.Background(), errors.New("test shutdown"))
	}()

	deadline := time.Now().Add(time.Second)
	for run.Context().Err() == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if run.Context().Err() == nil {
		t.Fatal("run context was not canceled")
	}
	if communication.CurrentRun() != run {
		t.Fatal("run ownership was released before its worker stopped")
	}
	if communication.TrySendMotion(MotionDataPartial{}) {
		t.Fatal("stopping run accepted new input")
	}
	if communication.MainQueue() != nil {
		t.Fatal("stopping run exposed its queue")
	}

	replacement := NewAgentRun(context.Background(), communication, false)
	if err := replacement.Activate(); !errors.Is(err, ErrAgentRunActive) {
		t.Fatalf("replacement Activate() error = %v, want ErrAgentRunActive", err)
	}
	replacement.Shutdown(context.Background(), errors.New("test complete"))

	close(release)
	report := <-shutdownDone
	if !report.Complete {
		t.Fatalf("shutdown report = %+v, want complete", report)
	}
	if communication.CurrentRun() != nil {
		t.Fatal("completed shutdown left the run attached")
	}
}

func TestAgentRunShutdownReportsStuckTask(t *testing.T) {
	communication := &Communication{}
	run := NewAgentRun(context.Background(), communication, false)
	release := make(chan struct{})
	started := make(chan struct{})
	if err := run.Go("blocked", lifecycle.TaskPolicy{}, func(context.Context) error {
		close(started)
		<-release
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := run.Activate(); err != nil {
		t.Fatal(err)
	}
	run.Seal()
	<-started

	waitContext, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	report := run.Shutdown(waitContext, errors.New("test shutdown"))
	if report.Complete {
		t.Fatal("shutdown unexpectedly completed")
	}
	if len(report.Running) != 1 || report.Running[0].Name != "blocked" {
		t.Fatalf("running tasks = %+v, want blocked", report.Running)
	}
	if communication.CurrentRun() != run {
		t.Fatal("timed-out shutdown released run ownership")
	}
	replacement := NewAgentRun(context.Background(), communication, false)
	if err := replacement.Activate(); !errors.Is(err, ErrAgentRunActive) {
		t.Fatalf("replacement Activate() error = %v, want ErrAgentRunActive", err)
	}
	replacement.Shutdown(context.Background(), errors.New("test complete"))
	close(release)
	waitContext, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	if followup := run.Wait(waitContext); !followup.Complete {
		t.Fatalf("follow-up report = %+v, want complete", followup)
	}
	communication.detachRun(run)
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
