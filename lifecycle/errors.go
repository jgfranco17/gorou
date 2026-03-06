package lifecycle

import (
	"errors"
	"fmt"
)

// ErrManagerAlreadyRunning is returned by Run when the manager is already
// running. It is a programming error to call Run concurrently.
var ErrManagerAlreadyRunning = errors.New("gorou: manager is already running")

// workerPanicError wraps a value recovered from a panic in a worker goroutine
// and presents it as an error so that the supervisor can handle it uniformly.
type workerPanicError struct {
	value any
}

func (e *workerPanicError) Error() string {
	return fmt.Sprintf("gorou: worker panicked: %v", e.value)
}
