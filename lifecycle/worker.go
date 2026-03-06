package lifecycle

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Worker is the public definition of a supervised goroutine. It is passed to
// Manager.Worker() to register a worker with a full configuration. For the
// common case of a no-restart worker, Manager.Go() is more convenient.
type Worker struct {
	// Name is a unique identifier for this worker within the manager. It must
	// be non-empty and must not be shared with any other registered worker.
	Name string

	// Run is the function executed by the worker goroutine. It receives the
	// manager's context and must respect cancellation by returning promptly when
	// ctx.Done() is closed. A nil return is a clean exit; a non-nil return is a
	// failure. Panics are recovered and converted to errors.
	Run func(ctx context.Context) error

	// Restart defines the restart behaviour when the worker exits.
	// Defaults to RestartNever.
	Restart RestartPolicy
}

// workerEntry is the internal runtime record for a registered worker. It pairs
// the public Worker definition with the mutable state tracked by the supervisor.
type workerEntry struct {
	Worker

	// state is accessed via sync/atomic so reads from Status() do not require
	// acquiring mu.
	state atomic.Int32

	// mu guards restartCount, lastError, and startedAt.
	mu           sync.RWMutex
	restartCount int
	lastError    error
	startedAt    time.Time
}

// setState atomically updates the worker's lifecycle state.
func (e *workerEntry) setState(s WorkerState) {
	e.state.Store(int32(s))
}

// getState atomically reads the worker's current lifecycle state.
func (e *workerEntry) getState() WorkerState {
	return WorkerState(e.state.Load())
}

// setError records the most recent error and, when the error is non-nil,
// updates the lastError field. A nil error does not clear a previous lastError
// so that callers can inspect the failure after the worker stops.
func (e *workerEntry) setError(err error) {
	if err == nil {
		return
	}
	e.mu.Lock()
	e.lastError = err
	e.mu.Unlock()
}

// markStarted records the time at which the worker's Run function was invoked.
func (e *workerEntry) markStarted() {
	e.mu.Lock()
	e.startedAt = time.Now()
	e.mu.Unlock()
}

// incrementRestartCount atomically increments the restart counter.
func (e *workerEntry) incrementRestartCount() {
	e.mu.Lock()
	e.restartCount++
	e.mu.Unlock()
}

// snapshot returns a point-in-time Status for this worker.
func (e *workerEntry) snapshot() Status {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return Status{
		Name:         e.Name,
		State:        e.getState(),
		RestartCount: e.restartCount,
		LastError:    e.lastError,
		StartedAt:    e.startedAt,
	}
}
