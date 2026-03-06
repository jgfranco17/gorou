package main

// Demonstrates graceful shutdown driven by OS signals (SIGINT / SIGTERM).
// Three workers run indefinitely.
// Send Ctrl-C to trigger a clean shutdown.
// Run: go run ./examples/graceful_shutdown

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jgfranco17/gorou/lifecycle"
)

func poller(name string, interval time.Duration) func(context.Context) error {
	return func(ctx context.Context) error {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Printf("[%s] received shutdown signal, stopping", name)
				return nil
			case <-ticker.C:
				log.Printf("[%s] polling", name)
			}
		}
	}
}

func main() {
	mgr := lifecycle.New()
	mgr.Go("fast-poller", poller("fast-poller", 500*time.Millisecond))
	mgr.Go("slow-poller", poller("slow-poller", 2*time.Second))
	mgr.Go("heartbeat", poller("heartbeat", time.Second))

	// Derive a context that is cancelled on SIGINT or SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Println("manager running - press Ctrl-C to stop")

	if err := mgr.Run(ctx); err != nil {
		log.Fatalf("manager exited with error: %v", err)
	}

	log.Println("shutdown complete")
}
