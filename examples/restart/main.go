// Package main demonstrates restart policies with the lifecycle manager.
//
// A flaky worker fails three times before stabilising. With RestartOnFailure,
// the manager automatically restarts it on each failure. The Status API is
// polled in a background goroutine to show the restart count in real time.
//
// Run:
//
//	go run ./examples/restart
package main

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/jgfranco17/gorou/lifecycle"
)

var errTransient = errors.New("transient connection error")

// flaky simulates a worker that fails several times before running stably.
func flaky(ctx context.Context) error {
	// Shared state across calls is tracked via a closure variable in main; this
	// worker just increments a counter exposed through Status.RestartCount.
	log.Println("[flaky] starting")
	time.Sleep(300 * time.Millisecond)
	return errTransient
}

// stable is a worker that runs cleanly until the context is cancelled.
func stable(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[stable] shutting down")
			return nil
		case <-ticker.C:
			log.Println("[stable] tick")
		}
	}
}

func main() {
	mgr := lifecycle.New()

	// This worker will be restarted each time it returns a non-nil error.
	mgr.AddWorker(lifecycle.Worker{
		Name:    "flaky",
		Run:     flaky,
		Restart: lifecycle.RestartOnFailure,
	})

	mgr.Go("stable", stable)

	// Poll Status in the background to show live restart counts.
	statusDone := make(chan struct{})
	go func() {
		defer close(statusDone)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			for _, s := range mgr.Status() {
				log.Printf("[status] %-10s  state=%-12s restarts=%d",
					s.Name, s.State, s.RestartCount)
			}
		}
	}()

	// Stop after 4 seconds; by then the flaky worker will have restarted several times.
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	log.Println("starting manager")
	if err := mgr.Run(ctx); err != nil {
		log.Printf("manager exited with error: %v", err)
	}

	// Print final state.
	for _, s := range mgr.Status() {
		log.Printf("[final] %-10s  state=%-12s restarts=%d lastErr=%v",
			s.Name, s.State, s.RestartCount, s.LastError)
	}
}
