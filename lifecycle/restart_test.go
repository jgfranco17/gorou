package lifecycle

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldRestart(t *testing.T) {
	sentinel := errors.New("failure")

	cases := []struct {
		name   string
		policy RestartPolicy
		err    error
		want   bool
	}{
		{"Never/nil", RestartNever, nil, false},
		{"Never/err", RestartNever, sentinel, false},
		{"OnFailure/nil", RestartOnFailure, nil, false},
		{"OnFailure/err", RestartOnFailure, sentinel, true},
		{"Always/nil", RestartAlways, nil, true},
		{"Always/err", RestartAlways, sentinel, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, shouldRestart(tc.policy, tc.err))
		})
	}
}

func TestRestartNever_WorkerErrorNotRestarted(t *testing.T) {
	sentinel := errors.New("failure")
	var calls atomic.Int32

	m := New()
	m.AddWorker(Worker{
		Name:    "w",
		Restart: RestartNever,
		Run: func(ctx context.Context) error {
			calls.Add(1)
			return sentinel
		},
	})

	err := m.Run(context.Background())
	assert.EqualValues(t, 1, calls.Load())
	assert.ErrorIs(t, err, sentinel)
}

func TestRestartOnFailure_RestartsOnError(t *testing.T) {
	const failTimes = 3
	var calls atomic.Int32

	m := New()
	m.AddWorker(Worker{
		Name:    "w",
		Restart: RestartOnFailure,
		Run: func(ctx context.Context) error {
			n := calls.Add(1)
			if n <= failTimes {
				return errors.New("transient error")
			}
			// Return nil on the 4th call — clean exit, stops restarting.
			return nil
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, m.Run(ctx))
	assert.EqualValues(t, failTimes+1, calls.Load())
}

func TestRestartOnFailure_NoRestartOnCleanExit(t *testing.T) {
	var calls atomic.Int32

	m := New()
	m.AddWorker(Worker{
		Name:    "w",
		Restart: RestartOnFailure,
		Run: func(ctx context.Context) error {
			calls.Add(1)
			return nil // clean exit — must not be restarted
		},
	})

	done := make(chan error, 1)
	go func() {
		done <- m.Run(context.Background())
	}()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return; worker may have been restarted incorrectly")
	}

	assert.EqualValues(t, 1, calls.Load())
}

func TestRestartAlways_RestartsOnCleanExit(t *testing.T) {
	var calls atomic.Int32
	ready := make(chan struct{})

	m := New()
	m.AddWorker(Worker{
		Name:    "w",
		Restart: RestartAlways,
		Run: func(ctx context.Context) error {
			if calls.Add(1) == 3 {
				close(ready)
			}
			// Block so the worker doesn't spin-loop; exit cleanly on cancel.
			select {
			case <-ctx.Done():
				return nil
			default:
				return nil // immediate clean exit — triggers RestartAlways
			}
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- m.Run(ctx)
	}()

	// Wait until the worker has been invoked at least 3 times, then stop.
	select {
	case <-ready:
		cancel()
	case <-time.After(5 * time.Second):
		t.Fatal("worker was not restarted 3 times within timeout")
	}

	assert.NoError(t, <-runDone)
	assert.GreaterOrEqual(t, calls.Load(), int32(3))
}

func TestRestartAlways_RestartsOnError(t *testing.T) {
	var calls atomic.Int32
	ready := make(chan struct{})

	m := New()
	m.AddWorker(Worker{
		Name:    "w",
		Restart: RestartAlways,
		Run: func(ctx context.Context) error {
			if calls.Add(1) == 3 {
				close(ready)
			}
			// Block briefly so that a pending ctx cancellation wins before we
			// return the error, keeping the shutdown path clean (nil return).
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(10 * time.Millisecond):
				return errors.New("always failing")
			}
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- m.Run(ctx)
	}()

	select {
	case <-ready:
		cancel()
	case <-time.After(5 * time.Second):
		t.Fatal("worker was not restarted 3 times within timeout")
	}

	assert.NoError(t, <-runDone)
}

func TestRestartOnFailure_RestartCountTracked(t *testing.T) {
	const failTimes = 4
	var calls atomic.Int32

	m := New()
	m.AddWorker(Worker{
		Name:    "w",
		Restart: RestartOnFailure,
		Run: func(ctx context.Context) error {
			if calls.Add(1) <= failTimes {
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
