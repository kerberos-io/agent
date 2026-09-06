package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sort"
	"sync"
	"time"
)

var (
	ErrSupervisorSealed  = errors.New("supervisor is sealed")
	ErrSupervisorStopped = errors.New("supervisor is shutting down")
	ErrTaskExists        = errors.New("task already exists")
)

type TaskFunc func(context.Context) error

type TaskPolicy struct {
	Required    bool
	LongRunning bool
}

type TaskStatus string

const (
	TaskRunning   TaskStatus = "running"
	TaskSucceeded TaskStatus = "succeeded"
	TaskFailed    TaskStatus = "failed"
	TaskPanicked  TaskStatus = "panicked"
)

type TaskSnapshot struct {
	Name      string
	Policy    TaskPolicy
	Status    TaskStatus
	StartedAt time.Time
	EndedAt   time.Time
	Error     string
	Panic     string
	Stack     []byte
}

type Failure struct {
	Task  string
	Cause error
	Panic string
	Stack []byte
}

func (f Failure) Error() string {
	if f.Cause == nil {
		return fmt.Sprintf("task %q failed", f.Task)
	}
	return f.Cause.Error()
}

type ShutdownReport struct {
	Complete bool
	Cause    error
	Tasks    []TaskSnapshot
	Running  []TaskSnapshot
}

type supervisorPhase uint8

const (
	phaseStarting supervisorPhase = iota
	phaseRunning
	phaseDraining
)

type taskState struct {
	TaskSnapshot
}

type Supervisor struct {
	ctx    context.Context
	cancel context.CancelCauseFunc

	mu        sync.Mutex
	phase     supervisorPhase
	sealed    bool
	active    int
	tasks     map[string]*taskState
	allDone   chan struct{}
	doneOnce  sync.Once
	start     chan struct{}
	startOnce sync.Once

	failures    chan Failure
	failureOnce sync.Once

	stopParentCancel func() bool
}

func NewSupervisor(parent context.Context) *Supervisor {
	if parent == nil {
		parent = context.Background()
	}

	ctx, cancel := context.WithCancelCause(parent)
	supervisor := &Supervisor{
		ctx:      ctx,
		cancel:   cancel,
		phase:    phaseStarting,
		tasks:    make(map[string]*taskState),
		allDone:  make(chan struct{}),
		start:    make(chan struct{}),
		failures: make(chan Failure, 1),
	}

	stopParentCancel := context.AfterFunc(parent, func() {
		supervisor.BeginShutdown(context.Cause(parent))
	})
	supervisor.mu.Lock()
	if supervisor.sealed && supervisor.active == 0 {
		supervisor.mu.Unlock()
		stopParentCancel()
	} else {
		supervisor.stopParentCancel = stopParentCancel
		supervisor.mu.Unlock()
	}

	return supervisor
}

func (s *Supervisor) Context() context.Context {
	return s.ctx
}

func (s *Supervisor) Failures() <-chan Failure {
	return s.failures
}

func (s *Supervisor) Go(name string, policy TaskPolicy, task TaskFunc) error {
	if name == "" {
		return errors.New("task name is required")
	}
	if task == nil {
		return errors.New("task function is required")
	}

	s.mu.Lock()
	if s.sealed {
		s.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrSupervisorSealed, name)
	}
	if s.ctx.Err() != nil {
		s.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrSupervisorStopped, name)
	}
	if _, exists := s.tasks[name]; exists {
		s.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrTaskExists, name)
	}

	state := &taskState{TaskSnapshot: TaskSnapshot{
		Name:      name,
		Policy:    policy,
		Status:    TaskRunning,
		StartedAt: time.Now(),
	}}
	s.tasks[name] = state
	s.active++
	s.mu.Unlock()

	go s.run(state, task)
	return nil
}

func (s *Supervisor) run(state *taskState, task TaskFunc) {
	var taskErr error
	panicked := true
	var panicValue string
	var panicStack []byte

	defer func() {
		recovered := recover()
		if panicked {
			panicValue = fmt.Sprint(recovered)
			panicStack = debug.Stack()
			taskErr = fmt.Errorf("task %q panicked: %s", state.Name, panicValue)
		}
		s.finish(state, taskErr, panicked, panicValue, panicStack)
	}()

	select {
	case <-s.start:
	case <-s.ctx.Done():
		panicked = false
		return
	}

	taskErr = task(s.ctx)
	panicked = false
}

func (s *Supervisor) finish(
	state *taskState,
	taskErr error,
	panicked bool,
	panicValue string,
	panicStack []byte,
) {
	var failure *Failure

	s.mu.Lock()
	state.EndedAt = time.Now()
	state.Error = ""
	state.Panic = panicValue
	state.Stack = append([]byte(nil), panicStack...)
	switch {
	case panicked:
		state.Status = TaskPanicked
		state.Error = taskErr.Error()
	case taskErr != nil:
		state.Status = TaskFailed
		state.Error = taskErr.Error()
	default:
		state.Status = TaskSucceeded
	}

	shuttingDown := s.phase == phaseDraining || s.ctx.Err() != nil
	unexpected := panicked || (!shuttingDown &&
		(state.Policy.Required && (taskErr != nil || state.Policy.LongRunning)))
	if unexpected {
		cause := taskErr
		if cause == nil {
			cause = fmt.Errorf("required long-running task %q exited", state.Name)
		}
		failure = &Failure{
			Task:  state.Name,
			Cause: cause,
			Panic: panicValue,
			Stack: append([]byte(nil), panicStack...),
		}
		s.phase = phaseDraining
		s.sealed = true
		s.cancel(failure.Cause)
		s.failureOnce.Do(func() {
			s.failures <- *failure
		})
	}

	s.active--
	s.closeDoneIfReadyLocked()
	s.mu.Unlock()
}

func (s *Supervisor) Seal() {
	s.mu.Lock()
	s.sealed = true
	if s.phase == phaseStarting {
		s.phase = phaseRunning
	}
	s.startOnce.Do(func() {
		close(s.start)
	})
	s.closeDoneIfReadyLocked()
	s.mu.Unlock()
}

func (s *Supervisor) BeginShutdown(cause error) {
	if cause == nil {
		cause = context.Canceled
	}

	s.mu.Lock()
	s.phase = phaseDraining
	s.sealed = true
	s.cancel(cause)
	s.closeDoneIfReadyLocked()
	s.mu.Unlock()
}

func (s *Supervisor) Wait(ctx context.Context) ShutdownReport {
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-s.allDone:
	case <-ctx.Done():
	}

	return s.report()
}

func (s *Supervisor) Snapshot() []TaskSnapshot {
	return s.report().Tasks
}

func (s *Supervisor) closeDoneIfReadyLocked() {
	if !s.sealed || s.active != 0 {
		return
	}
	if s.stopParentCancel != nil {
		s.stopParentCancel()
		s.stopParentCancel = nil
	}
	s.doneOnce.Do(func() {
		close(s.allDone)
	})
}

func (s *Supervisor) report() ShutdownReport {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks := make([]TaskSnapshot, 0, len(s.tasks))
	running := make([]TaskSnapshot, 0)
	for _, state := range s.tasks {
		snapshot := state.TaskSnapshot
		snapshot.Stack = append([]byte(nil), state.Stack...)
		tasks = append(tasks, snapshot)
		if snapshot.Status == TaskRunning {
			running = append(running, snapshot)
		}
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].Name < tasks[j].Name
	})
	sort.Slice(running, func(i, j int) bool {
		return running[i].Name < running[j].Name
	})

	return ShutdownReport{
		Complete: s.sealed && s.active == 0,
		Cause:    context.Cause(s.ctx),
		Tasks:    tasks,
		Running:  running,
	}
}
