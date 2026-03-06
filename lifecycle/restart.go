package lifecycle

// RestartPolicy defines how a worker is treated after it exits.
type RestartPolicy int

const (
	// RestartNever means the worker is not restarted regardless of how it exits.
	// This is the zero value and the default for workers registered via Go().
	RestartNever RestartPolicy = iota

	// RestartOnFailure restarts the worker only when it exits with a non-nil error
	// or due to a panic. A nil return is treated as an intentional exit.
	RestartOnFailure

	// RestartAlways restarts the worker unconditionally after every exit,
	// regardless of whether it returned nil or an error.
	RestartAlways
)

// shouldRestart reports whether a worker with the given policy should be
// restarted after exiting with err. A nil err means the worker returned
// cleanly; a non-nil err means it failed or panicked.
func shouldRestart(policy RestartPolicy, err error) bool {
	switch policy {
	case RestartAlways:
		return true
	case RestartOnFailure:
		return err != nil
	default: // RestartNever
		return false
	}
}
