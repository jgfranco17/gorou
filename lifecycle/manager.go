package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

// Manager supervises a set of registered workers. It starts them, tracks their
// lifecycle states, applies restart policies, and coordinates graceful shutdown.
//
// A Manager must not be copied after first use.
type Manager struct {
	mu      sync.Mutex
	workers []*workerEntry
	names   map[string]struct{}
	running atomic.Bool
}

// New returns an initialised, empty Manager ready to accept worker
// registrations.
func New() *Manager {
	return &Manager{
		names: make(map[string]struct{}),
	}
}

// Go registers a worker that runs fn under the given name using RestartNever.
// It is a convenience wrapper around Worker for the common case.
//
// Go panics if:
//   - name is empty
//   - fn is nil
//   - a worker with the same name is already registered
//   - the manager is currently running
func (m *Manager) Go(name string, fn func(context.Context) error) {
	m.register(Worker{Name: name, Run: fn, Restart: RestartNever})
}

// AddWorker registers a worker with a full configuration. It panics under the
// same conditions as Go.
func (m *Manager) AddWorker(w Worker) {
	m.register(w)
}

// Run starts all registered workers and blocks until all have exited.
//
// Shutdown is triggered when ctx is cancelled; all workers receive the
// cancellation and are expected to return promptly. If all workers exit
// naturally (without a restart policy keeping them alive) before ctx is
// cancelled, Run returns early.
//
// Run returns a joined error of all terminal worker failures. Workers that exit
// cleanly (nil error) or that return context.Canceled / context.DeadlineExceeded
// during shutdown do not contribute to the returned error.
//
// Run returns ErrManagerAlreadyRunning if called while already running.
func (m *Manager) Run(ctx context.Context) error {
	if !m.running.CompareAndSwap(false, true) {
		return ErrManagerAlreadyRunning
	}
	defer m.running.Store(false)

	m.mu.Lock()
	workers := make([]*workerEntry, len(m.workers))
	copy(workers, m.workers)
	m.mu.Unlock()

	if len(workers) == 0 {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// resultCh carries terminal errors from workers that will not be restarted.
	// Buffer equals worker count so supervisor goroutines never block on send.
	resultCh := make(chan error, len(workers))

	var wg sync.WaitGroup
	for _, entry := range workers {
		wg.Add(1)
		go m.supervise(ctx, entry, resultCh, &wg)
	}

	// When all workers are done, cancel to ensure Run unblocks even if the
	// caller's context is still active.
	go func() {
		wg.Wait()
		cancel()
	}()

	wg.Wait()
	close(resultCh)

	var errs []error
	for err := range resultCh {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// Status returns a point-in-time snapshot of all registered workers. It is
// safe to call concurrently with Run.
func (m *Manager) Status() []Status {
	m.mu.Lock()
	workers := make([]*workerEntry, len(m.workers))
	copy(workers, m.workers)
	m.mu.Unlock()

	statuses := make([]Status, len(workers))
	for i, e := range workers {
		statuses[i] = e.snapshot()
	}
	return statuses
}

// register is the shared registration path for both Go and AddWorker.
func (m *Manager) register(w Worker) {
	if m.running.Load() {
		panic("lifecycle: cannot register worker while manager is running")
	}
	if w.Name == "" {
		panic("lifecycle: worker name must not be empty")
	}
	if w.Run == nil {
		panic("lifecycle: worker Run function must not be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.names[w.Name]; exists {
		panic(fmt.Sprintf("lifecycle: worker %q is already registered", w.Name))
	}

	entry := &workerEntry{Worker: w}
	entry.setState(StateStarting)
	m.workers = append(m.workers, entry)
	m.names[w.Name] = struct{}{}
}

// supervise runs the worker's Run function in a loop, applying the restart
// policy on each exit. Terminal errors (no restart) are sent to resultCh.
// Done is called on wg when the worker reaches a terminal state.
func (m *Manager) supervise(ctx context.Context, entry *workerEntry, resultCh chan<- error, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		entry.markStarted()
		entry.setState(StateRunning)

		err := runGuarded(ctx, entry.Run)

		if ctx.Err() != nil {
			// Shutdown path: context was cancelled. Errors that ARE the context
			// error are expected during graceful shutdown and are not failures.
			entry.setError(err)
			entry.setState(StateStopping)
			if err != nil && !errors.Is(err, ctx.Err()) {
				entry.setState(StateFailed)
				resultCh <- err
			} else {
				entry.setState(StateStopped)
			}
			return
		}

		// Context still active: apply the restart policy.
		if shouldRestart(entry.Restart, err) {
			entry.setError(err)
			entry.incrementRestartCount()
			entry.setState(StateRestarting)
			continue
		}

		// No restart: terminal exit.
		entry.setError(err)
		if err != nil {
			entry.setState(StateFailed)
			resultCh <- err
		} else {
			entry.setState(StateStopped)
		}
		return
	}
}

// runGuarded calls fn(ctx) and converts any panic into a *workerPanicError.
func runGuarded(ctx context.Context, fn func(context.Context) error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = &workerPanicError{value: r}
		}
	}()
	return fn(ctx)
}
