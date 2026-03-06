// Package main demonstrates basic usage of the lifecycle manager.
//
// Two workers run concurrently. The program stops after 5 seconds via a
// timeout context, at which point both workers shut down gracefully.
//
// Run:
//
//	go run ./examples/basic
package main

import (
	"context"
	"log"
	"time"

	"github.com/jgfranco17/gorou/lifecycle"
)

func metricsWorker(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[metrics] shutting down")
			return nil
		case t := <-ticker.C:
			log.Printf("[metrics] tick at %s", t.Format(time.TimeOnly))
		}
	}
}

func cacheRefreshWorker(ctx context.Context) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[cache] shutting down")
			return nil
		case <-ticker.C:
			log.Println("[cache] refreshed")
		}
	}
}

func main() {
	mgr := lifecycle.New()
	mgr.Go("metrics", metricsWorker)
	mgr.Go("cache-refresh", cacheRefreshWorker)

	// Run for 5 seconds then shut down.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Println("starting manager")
	if err := mgr.Run(ctx); err != nil {
		log.Fatalf("manager exited with error: %v", err)
	}
	log.Println("manager stopped cleanly")
}
