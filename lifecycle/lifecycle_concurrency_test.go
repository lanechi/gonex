package lifecycle

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestConcurrentShutdownAndStopReturnPhaseInProgress(t *testing.T) {
	lifecycle := New()
	shutdownStarted := make(chan struct{})
	releaseShutdown := make(chan struct{})
	stopCalled := make(chan struct{}, 1)
	var mu sync.Mutex
	order := make([]string, 0, 2)

	lifecycle.OnShutdown(func(context.Context) error {
		close(shutdownStarted)
		<-releaseShutdown
		mu.Lock()
		order = append(order, "shutdown")
		mu.Unlock()
		return nil
	})
	lifecycle.OnStop(func(context.Context) error {
		stopCalled <- struct{}{}
		mu.Lock()
		order = append(order, "stop")
		mu.Unlock()
		return nil
	})

	firstDone := make(chan error, 1)
	go func() { firstDone <- lifecycle.BeginShutdown(context.Background()) }()
	<-shutdownStarted

	if err := lifecycle.BeginShutdown(context.Background()); !errors.Is(err, ErrPhaseInProgress) {
		t.Fatalf("concurrent BeginShutdown error = %v, want ErrPhaseInProgress", err)
	}
	if err := lifecycle.Stop(context.Background()); !errors.Is(err, ErrPhaseInProgress) {
		t.Fatalf("Stop during shutdown error = %v, want ErrPhaseInProgress", err)
	}
	select {
	case <-stopCalled:
		t.Fatal("OnStop ran before active shutdown completed")
	default:
	}

	close(releaseShutdown)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != "shutdown" || order[1] != "stop" {
		t.Fatalf("unexpected lifecycle order: %v", order)
	}
}

func TestConcurrentStartReturnsPhaseInProgressWithoutWaiting(t *testing.T) {
	lifecycle := New()
	started := make(chan struct{})
	release := make(chan struct{})
	lifecycle.OnStart(func(context.Context) error {
		close(started)
		<-release
		return nil
	})
	first := make(chan error, 1)
	go func() { first <- lifecycle.BeginStart(context.Background()) }()
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := lifecycle.BeginStart(ctx); !errors.Is(err, ErrPhaseInProgress) {
		t.Fatalf("concurrent start error = %v, want ErrPhaseInProgress", err)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
}

func TestTrackedTasksCancelAndWaitWithoutWaitGroupOrdering(t *testing.T) {
	lifecycle := New()
	started := make(chan struct{})
	finished := make(chan struct{})
	lifecycle.Go(func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(finished)
	})
	<-started
	if err := lifecycle.BeginShutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := lifecycle.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	default:
		t.Fatal("tracked task was not canceled")
	}
}
