package lifecycle

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestConcurrentShutdownWaitsForSameAttemptAndStopWaitsForShutdown(t *testing.T) {
	l := New()
	shutdownStarted := make(chan struct{})
	releaseShutdown := make(chan struct{})
	var mu sync.Mutex
	order := make([]string, 0, 2)

	l.OnShutdown(func(context.Context) error {
		close(shutdownStarted)
		<-releaseShutdown
		mu.Lock()
		order = append(order, "shutdown")
		mu.Unlock()
		return nil
	})
	l.OnStop(func(context.Context) error {
		mu.Lock()
		order = append(order, "stop")
		mu.Unlock()
		return nil
	})

	firstDone := make(chan error, 1)
	go func() { firstDone <- l.BeginShutdown(context.Background()) }()
	<-shutdownStarted

	secondDone := make(chan error, 1)
	go func() { secondDone <- l.BeginShutdown(context.Background()) }()
	stopDone := make(chan error, 1)
	go func() { stopDone <- l.Stop(context.Background()) }()

	select {
	case <-secondDone:
		t.Fatal("concurrent BeginShutdown returned before active shutdown attempt completed")
	case <-stopDone:
		t.Fatal("Stop returned before active shutdown attempt completed")
	case <-time.After(20 * time.Millisecond):
	}

	close(releaseShutdown)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	if err := <-stopDone; err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != "shutdown" || order[1] != "stop" {
		t.Fatalf("unexpected lifecycle order: %v", order)
	}
}
