package lifecycle

import (
	"context"
	"errors"
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

func TestConcurrentStartWaiterHonorsContextCancellation(t *testing.T) {
	l := New()
	started := make(chan struct{})
	release := make(chan struct{})
	l.OnStart(func(context.Context) error {
		close(started)
		<-release
		return nil
	})
	first := make(chan error, 1)
	go func() { first <- l.BeginStart(context.Background()) }()
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := l.BeginStart(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("concurrent start wait error = %v", err)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
}

func TestStopTimeoutDoesNotRunFinalHooksBeforeShutdownCompletes(t *testing.T) {
	l := New()
	shutdownStarted := make(chan struct{})
	release := make(chan struct{})
	stopCalled := make(chan struct{}, 1)
	l.OnShutdown(func(context.Context) error {
		close(shutdownStarted)
		<-release
		return nil
	})
	l.OnStop(func(context.Context) error {
		stopCalled <- struct{}{}
		return nil
	})
	first := make(chan error, 1)
	go func() { first <- l.BeginShutdown(context.Background()) }()
	<-shutdownStarted

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := l.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop timeout error = %v", err)
	}
	select {
	case <-stopCalled:
		t.Fatal("OnStop ran before active shutdown completed")
	default:
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := l.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopCalled:
	default:
		t.Fatal("OnStop did not run after shutdown completed")
	}
}

func TestTrackedTasksCancelAndWaitWithoutWaitGroupOrdering(t *testing.T) {
	l := New()
	started := make(chan struct{})
	finished := make(chan struct{})
	l.Go(func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(finished)
	})
	<-started
	if err := l.BeginShutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := l.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	default:
		t.Fatal("tracked task was not canceled")
	}
}
