package lifecycle

import "time"

// WorkerState represents the lifecycle state of a supervised worker.
type WorkerState int32

const (
	// StateStarting indicates the worker has been registered and is being
	// initialised but has not yet begun executing its Run function.
	StateStarting WorkerState = iota

	// StateRunning indicates the worker's Run function is actively executing.
	StateRunning

	// StateStopping indicates the worker's context has been cancelled and the
	// supervisor is waiting for the Run function to return.
	StateStopping

	// StateStopped indicates the worker exited cleanly with a nil error and will
	// not be restarted.
	StateStopped

	// StateFailed indicates the worker exited with a non-nil error (or panic) and
	// will not be restarted.
	StateFailed

	// StateRestarting indicates the worker exited and is being restarted according
	// to its RestartPolicy.
	StateRestarting
)

// String returns a human-readable label for the state.
func (s WorkerState) String() string {
	switch s {
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateStopping:
		return "stopping"
	case StateStopped:
		return "stopped"
	case StateFailed:
		return "failed"
	case StateRestarting:
		return "restarting"
	default:
		return "unknown"
	}
}

// Status is a point-in-time snapshot of a worker's runtime state. It is safe
// to read after Run returns or while Run is executing.
type Status struct {
	// Name is the unique name the worker was registered with.
	Name string

	// State is the current lifecycle state of the worker.
	State WorkerState

	// RestartCount is the number of times the worker has been restarted.
	RestartCount int

	// LastError is the most recent non-nil error returned by the worker's Run
	// function, or nil if the worker has not yet failed.
	LastError error

	// StartedAt is the time the worker's Run function was most recently invoked.
	// It is the zero time if the worker has not yet started.
	StartedAt time.Time
}
