package lifecycle

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// block is a worker that blocks until its context is cancelled, then returns nil.
func block(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

// failWith returns a worker function that always returns err.
func failWith(err error) func(context.Context) error {
	return func(ctx context.Context) error {
		return err
	}
}

// syncedWorker returns a worker whose execution can be coordinated from a test.
// The worker signals ready on startCh, then waits for a signal on proceedCh
// before returning returnErr.
func syncedWorker(startCh chan<- struct{}, proceedCh <-chan struct{}, returnErr error) func(context.Context) error {
	return func(ctx context.Context) error {
		startCh <- struct{}{}
		select {
		case <-proceedCh:
			return returnErr
		case <-ctx.Done():
			return nil
		}
	}
}

func TestNew(t *testing.T) {
	m := New()
	require.NotNil(t, m)
	assert.Empty(t, m.workers)
}

func TestManagerRun_NoWorkers(t *testing.T) {
	m := New()
	assert.NoError(t, m.Run(context.Background()))
}

func TestManagerRun_BasicStartStop(t *testing.T) {
	m := New()
	m.Go("worker", block)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.NoError(t, m.Run(ctx))
}

func TestManagerRun_MultipleWorkers(t *testing.T) {
	m := New()
	m.Go("w1", block)
	m.Go("w2", block)
	m.Go("w3", block)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- m.Run(ctx)
	}()

	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}

func TestManagerRun_WorkerError(t *testing.T) {
	sentinel := errors.New("worker failed")
	m := New()
	m.Go("failing", failWith(sentinel))

	err := m.Run(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

func TestManagerRun_AllExitNaturally(t *testing.T) {
	// Workers that return nil immediately should cause Run to return without
	// needing context cancellation.
	m := New()
	for i := 0; i < 3; i++ {
		m.Go("w"+string(rune('0'+i)), func(ctx context.Context) error { return nil })
	}

	done := make(chan error, 1)
	go func() {
		done <- m.Run(context.Background())
	}()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after workers exited naturally")
	}
}

func TestManagerRun_AlreadyRunning(t *testing.T) {
	startCh := make(chan struct{})
	proceedCh := make(chan struct{})

	m := New()
	m.Go("w", syncedWorker(startCh, proceedCh, nil))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errs := make(chan error, 2)
	var wg sync.WaitGroup

	// First Run — launches the worker.
	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- m.Run(ctx)
	}()

	// Wait until the worker is running before attempting the second Run.
	<-startCh

	// Second Run — should fail immediately.
	wg.Add(1)
	go func() {
		defer wg.Done()
		errs <- m.Run(ctx)
	}()

	// Collect the error from whichever Run returns first; it must be
	// ErrManagerAlreadyRunning.
	select {
	case err := <-errs:
		assert.ErrorIs(t, err, ErrManagerAlreadyRunning)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for second Run to return")
	}

	// Clean up.
	close(proceedCh)
	cancel()
	wg.Wait()
}

func TestManagerRun_PanicRecovery(t *testing.T) {
	m := New()
	m.Go("panicky", func(ctx context.Context) error {
		panic("something went wrong")
	})

	err := m.Run(context.Background())
	require.Error(t, err)
	var pe *workerPanicError
	assert.ErrorAs(t, err, &pe)
}

func TestManagerGo_PanicOnEmptyName(t *testing.T) {
	m := New()
	assert.Panics(t, func() {
		m.Go("", func(ctx context.Context) error { return nil })
	})
}

func TestManagerGo_PanicOnNilFn(t *testing.T) {
	m := New()
	assert.Panics(t, func() {
		m.Go("worker", nil)
	})
}

func TestManagerGo_PanicOnDuplicate(t *testing.T) {
	m := New()
	assert.Panics(t, func() {
		m.Go("worker", block)
		m.Go("worker", block)
	})
}

func TestManagerAddWorker_PanicOnDuplicate(t *testing.T) {
	m := New()
	w := Worker{Name: "worker", Run: block}
	assert.Panics(t, func() {
		m.AddWorker(w)
		m.AddWorker(w)
	})
}

func TestManagerRun_MultipleWorkerErrors(t *testing.T) {
	errA := errors.New("error-a")
	errB := errors.New("error-b")

	m := New()
	m.Go("a", failWith(errA))
	m.Go("b", failWith(errB))

	err := m.Run(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, errA)
	assert.ErrorIs(t, err, errB)
}

func TestManagerRun_ContextCancelIsNotError(t *testing.T) {
	// A worker that returns ctx.Err() on shutdown must not pollute the Run result.
	m := New()
	m.Go("w", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err() // context.Canceled — expected, not a failure
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.NoError(t, m.Run(ctx))
}
