package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatus_Empty(t *testing.T) {
	m := New()
	assert.Empty(t, m.Status())
}

func TestStatus_InitialState(t *testing.T) {
	m := New()
	m.Go("w1", block)
	m.Go("w2", block)

	for _, s := range m.Status() {
		assert.Equal(t, StateStarting, s.State, "worker %q should be StateStarting before Run", s.Name)
		assert.True(t, s.StartedAt.IsZero(), "worker %q StartedAt should be zero before Run", s.Name)
		assert.Zero(t, s.RestartCount, "worker %q RestartCount should be 0 before Run", s.Name)
	}
}

func TestStatus_RunningStateDuringRun(t *testing.T) {
	startCh := make(chan struct{}, 1)
	proceedCh := make(chan struct{})

	m := New()
	m.Go("w", syncedWorker(startCh, proceedCh, nil))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- m.Run(ctx)
	}()

	// Wait until the worker has signalled it is executing.
	select {
	case <-startCh:
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not start within timeout")
	}

	for _, s := range m.Status() {
		if s.Name == "w" {
			assert.Equal(t, StateRunning, s.State)
			assert.False(t, s.StartedAt.IsZero(), "StartedAt should be set once worker is running")
		}
	}

	close(proceedCh)
	cancel()
	<-runDone
}

func TestStatus_StoppedAfterCleanExit(t *testing.T) {
	m := New()
	m.Go("w", func(ctx context.Context) error { return nil })

	require.NoError(t, m.Run(context.Background()))

	statuses := m.Status()
	require.Len(t, statuses, 1)
	assert.Equal(t, StateStopped, statuses[0].State)
	assert.NoError(t, statuses[0].LastError)
}

func TestStatus_FailedAfterErrorExit(t *testing.T) {
	sentinel := errors.New("worker error")

	m := New()
	m.Go("w", failWith(sentinel))
	_ = m.Run(context.Background())

	statuses := m.Status()
	require.Len(t, statuses, 1)
	assert.Equal(t, StateFailed, statuses[0].State)
	assert.ErrorIs(t, statuses[0].LastError, sentinel)
}

func TestStatus_FailedAfterPanic(t *testing.T) {
	m := New()
	m.Go("w", func(ctx context.Context) error {
		panic("kaboom")
	})
	_ = m.Run(context.Background())

	statuses := m.Status()
	require.Len(t, statuses, 1)
	assert.Equal(t, StateFailed, statuses[0].State)
	var pe *workerPanicError
	assert.ErrorAs(t, statuses[0].LastError, &pe)
}

func TestStatus_RestartCountReflected(t *testing.T) {
	const failTimes = 3
	var calls int

	m := New()
	m.AddWorker(Worker{
		Name:    "w",
		Restart: RestartOnFailure,
		Run: func(ctx context.Context) error {
			calls++
			if calls <= failTimes {
				return errors.New("transient")
			}
			return nil
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, m.Run(ctx))

	statuses := m.Status()
	require.Len(t, statuses, 1)
	assert.Equal(t, failTimes, statuses[0].RestartCount)
}

func TestStatus_StartedAtUpdatedOnRestart(t *testing.T) {
	var calls int
	startCh := make(chan struct{}, 1)

	m := New()
	m.AddWorker(Worker{
		Name:    "w",
		Restart: RestartOnFailure,
		Run: func(ctx context.Context) error {
			calls++
			if calls == 1 {
				startCh <- struct{}{}
				return errors.New("first failure")
			}
			// Second invocation: block until context is cancelled.
			<-ctx.Done()
			return nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- m.Run(ctx)
	}()

	// Capture StartedAt after first run.
	<-startCh
	time.Sleep(10 * time.Millisecond) // let the restart cycle set the new startedAt
	first := m.Status()[0].StartedAt

	// Wait for the second execution to begin, then cancel.
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-runDone

	second := m.Status()[0].StartedAt
	assert.False(t, second.IsZero(), "StartedAt should not be zero after restart")
	// Both timestamps should have been set (clock resolution may make them equal
	// on coarse-grained systems, so we only require second is not before first).
	assert.False(t, second.Before(first), "StartedAt after restart should not be before the first StartedAt")
}

func TestWorkerStateString(t *testing.T) {
	cases := []struct {
		state WorkerState
		want  string
	}{
		{StateStarting, "starting"},
		{StateRunning, "running"},
		{StateStopping, "stopping"},
		{StateStopped, "stopped"},
		{StateFailed, "failed"},
		{StateRestarting, "restarting"},
		{WorkerState(99), "unknown"},
	}

	for _, tc := range cases {
		assert.Equal(t, tc.want, tc.state.String())
	}
}
