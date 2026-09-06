package lifecycle

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSupervisorWaitsForSeal(t *testing.T) {
	supervisor := NewSupervisor(context.Background())
	started := make(chan struct{})
	if err := supervisor.Go("quick", TaskPolicy{}, func(context.Context) error {
		close(started)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	waitContext, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if report := supervisor.Wait(waitContext); report.Complete {
		t.Fatal("Wait() completed before Seal()")
	}
	select {
	case <-started:
		t.Fatal("task started before Seal()")
	default:
	}

	supervisor.Seal()
	<-started
	if report := supervisor.Wait(context.Background()); !report.Complete {
		t.Fatal("Wait() did not complete after Seal()")
	}
}

func TestSupervisorSealsEmptyGroup(t *testing.T) {
	supervisor := NewSupervisor(context.Background())
	supervisor.Seal()

	report := supervisor.Wait(context.Background())
	if !report.Complete || len(report.Tasks) != 0 {
		t.Fatalf("Wait() report = %+v, want complete empty report", report)
	}
}

func TestSupervisorReportsRunningTasksOnTimeout(t *testing.T) {
	supervisor := NewSupervisor(context.Background())
	release := make(chan struct{})
	if err := supervisor.Go("blocked", TaskPolicy{}, func(context.Context) error {
		<-release
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	supervisor.Seal()

	waitContext, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	report := supervisor.Wait(waitContext)
	if report.Complete {
		t.Fatal("Wait() unexpectedly completed")
	}
	if len(report.Running) != 1 || report.Running[0].Name != "blocked" {
		t.Fatalf("Wait() running tasks = %+v, want blocked", report.Running)
	}

	close(release)
	if report = supervisor.Wait(context.Background()); !report.Complete {
		t.Fatal("Wait() did not complete after releasing task")
	}
}

func TestSupervisorFailsWhenRequiredLongRunningTaskExits(t *testing.T) {
	supervisor := NewSupervisor(context.Background())
	release := make(chan struct{})
	if err := supervisor.Go("recorder", TaskPolicy{Required: true, LongRunning: true}, func(context.Context) error {
		<-release
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	supervisor.Seal()
	close(release)

	select {
	case failure := <-supervisor.Failures():
		if failure.Task != "recorder" || !strings.Contains(failure.Error(), "exited") {
			t.Fatalf("failure = %+v", failure)
		}
	case <-time.After(time.Second):
		t.Fatal("required task exit did not emit a failure")
	}
}

func TestSupervisorAllowsRequiredOneShotTaskToSucceed(t *testing.T) {
	supervisor := NewSupervisor(context.Background())
	if err := supervisor.Go("setup", TaskPolicy{Required: true}, func(context.Context) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	supervisor.Seal()
	if report := supervisor.Wait(context.Background()); !report.Complete {
		t.Fatal("Wait() did not complete")
	}

	select {
	case failure := <-supervisor.Failures():
		t.Fatalf("successful one-shot task emitted failure: %+v", failure)
	default:
	}
}

func TestSupervisorFailsWhenRequiredOneShotTaskReturnsError(t *testing.T) {
	supervisor := NewSupervisor(context.Background())
	wantErr := errors.New("setup failed")
	if err := supervisor.Go("setup", TaskPolicy{Required: true}, func(context.Context) error {
		return wantErr
	}); err != nil {
		t.Fatal(err)
	}
	supervisor.Seal()

	select {
	case failure := <-supervisor.Failures():
		if !errors.Is(failure.Cause, wantErr) {
			t.Fatalf("failure cause = %v, want %v", failure.Cause, wantErr)
		}
	case <-time.After(time.Second):
		t.Fatal("required task error did not emit a failure")
	}
}

func TestSupervisorRecoversPanicAndCapturesStack(t *testing.T) {
	supervisor := NewSupervisor(context.Background())
	if err := supervisor.Go("panic", TaskPolicy{}, func(context.Context) error {
		panic("boom")
	}); err != nil {
		t.Fatal(err)
	}
	supervisor.Seal()

	select {
	case failure := <-supervisor.Failures():
		if failure.Task != "panic" || failure.Panic != "boom" || len(failure.Stack) == 0 {
			t.Fatalf("failure = %+v, want captured panic and stack", failure)
		}
	case <-time.After(time.Second):
		t.Fatal("panic did not emit a failure")
	}

	report := supervisor.Wait(context.Background())
	if !report.Complete || len(report.Tasks) != 1 || len(report.Tasks[0].Stack) == 0 {
		t.Fatalf("report = %+v, want completed task with captured stack", report)
	}
}

func TestSupervisorCapturesEmptyPanicDuringShutdown(t *testing.T) {
	supervisor := NewSupervisor(context.Background())
	started := make(chan struct{})
	if err := supervisor.Go("panic", TaskPolicy{}, func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		panic("")
	}); err != nil {
		t.Fatal(err)
	}
	supervisor.Seal()
	<-started
	supervisor.BeginShutdown(errors.New("requested shutdown"))

	select {
	case failure := <-supervisor.Failures():
		if failure.Task != "panic" || len(failure.Stack) == 0 {
			t.Fatalf("failure = %+v, want captured panic and stack", failure)
		}
	case <-time.After(time.Second):
		t.Fatal("panic during shutdown did not emit a failure")
	}
	if report := supervisor.Wait(context.Background()); !report.Complete || report.Tasks[0].Status != TaskPanicked {
		t.Fatalf("report = %+v, want completed panicked task", report)
	}
}

func TestSupervisorPublishesFailureBeforeCompletion(t *testing.T) {
	supervisor := NewSupervisor(context.Background())
	wantErr := errors.New("setup failed")
	if err := supervisor.Go("setup", TaskPolicy{Required: true}, func(context.Context) error {
		return wantErr
	}); err != nil {
		t.Fatal(err)
	}
	supervisor.Seal()

	report := supervisor.Wait(context.Background())
	if !report.Complete || !errors.Is(report.Cause, wantErr) {
		t.Fatalf("report = %+v, want complete report caused by task failure", report)
	}
	select {
	case failure := <-supervisor.Failures():
		if !errors.Is(failure.Cause, wantErr) {
			t.Fatalf("failure cause = %v, want %v", failure.Cause, wantErr)
		}
	default:
		t.Fatal("failure was not published before completion")
	}
}

func TestSupervisorCompletedWaitWinsOverCanceledContext(t *testing.T) {
	supervisor := NewSupervisor(context.Background())
	supervisor.Seal()

	waitContext, cancel := context.WithCancel(context.Background())
	cancel()
	if report := supervisor.Wait(waitContext); !report.Complete {
		t.Fatalf("report = %+v, want authoritative completion state", report)
	}
}

func TestSupervisorRejectsTaskAfterParentCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	supervisor := NewSupervisor(parent)
	cancel()

	err := supervisor.Go("late", TaskPolicy{}, func(context.Context) error { return nil })
	if !errors.Is(err, ErrSupervisorStopped) && !errors.Is(err, ErrSupervisorSealed) {
		t.Fatalf("Go() error = %v, want stopped or sealed supervisor", err)
	}
	supervisor.BeginShutdown(context.Canceled)
	if report := supervisor.Wait(context.Background()); !report.Complete {
		t.Fatalf("report = %+v, want complete canceled supervisor", report)
	}
}

func TestSupervisorShutdownBeforeSealDoesNotStartTasks(t *testing.T) {
	supervisor := NewSupervisor(context.Background())
	started := make(chan struct{})
	if err := supervisor.Go("worker", TaskPolicy{}, func(context.Context) error {
		close(started)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	supervisor.BeginShutdown(errors.New("startup aborted"))
	if report := supervisor.Wait(context.Background()); !report.Complete {
		t.Fatalf("report = %+v, want completed aborted startup", report)
	}
	select {
	case <-started:
		t.Fatal("task started after startup was aborted")
	default:
	}
}

func TestSupervisorRejectsDuplicateAndLateTasks(t *testing.T) {
	supervisor := NewSupervisor(context.Background())
	release := make(chan struct{})
	if err := supervisor.Go("worker", TaskPolicy{}, func(context.Context) error {
		<-release
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := supervisor.Go("worker", TaskPolicy{}, func(context.Context) error { return nil }); !errors.Is(err, ErrTaskExists) {
		t.Fatalf("duplicate Go() error = %v, want ErrTaskExists", err)
	}

	supervisor.Seal()
	if err := supervisor.Go("late", TaskPolicy{}, func(context.Context) error { return nil }); !errors.Is(err, ErrSupervisorSealed) {
		t.Fatalf("late Go() error = %v, want ErrSupervisorSealed", err)
	}
	close(release)
}

func TestSupervisorConcurrentStartAndSeal(t *testing.T) {
	supervisor := NewSupervisor(context.Background())
	var starters sync.WaitGroup
	for index := 0; index < 100; index++ {
		starters.Add(1)
		go func(index int) {
			defer starters.Done()
			_ = supervisor.Go(
				"worker-"+time.Unix(0, int64(index)).Format("150405.000000000"),
				TaskPolicy{},
				func(context.Context) error { return nil },
			)
		}(index)
	}

	supervisor.Seal()
	starters.Wait()
	if report := supervisor.Wait(context.Background()); !report.Complete {
		t.Fatal("Wait() did not complete after concurrent Start and Seal")
	}
}
