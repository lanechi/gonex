package lifecycle

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestShutdownWaitsForActiveStartHooks(t *testing.T) {
	manager := New()
	startEntered := make(chan struct{})
	releaseStart := make(chan struct{})
	shutdownRan := make(chan struct{}, 1)
	var mu sync.Mutex
	order := make([]string, 0, 2)

	manager.OnStart(func(context.Context) error {
		close(startEntered)
		<-releaseStart
		mu.Lock()
		order = append(order, "start")
		mu.Unlock()
		return nil
	})
	manager.OnShutdown(func(context.Context) error {
		mu.Lock()
		order = append(order, "shutdown")
		mu.Unlock()
		shutdownRan <- struct{}{}
		return nil
	})

	startDone := make(chan error, 1)
	go func() { startDone <- manager.BeginStart(context.Background()) }()
	<-startEntered

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- manager.BeginShutdown(context.Background()) }()
	select {
	case <-shutdownRan:
		t.Fatal("shutdown hook ran while start hook was active")
	case <-time.After(20 * time.Millisecond):
	}

	close(releaseStart)
	if err := <-startDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("start error = %v, want context.Canceled", err)
	}
	if err := <-shutdownDone; err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != "start" || order[1] != "shutdown" {
		t.Fatalf("phase order = %v, want [start shutdown]", order)
	}
}

func TestShutdownTimeoutWhileStartActiveCanBeRetried(t *testing.T) {
	manager := New()
	startEntered := make(chan struct{})
	releaseStart := make(chan struct{})
	shutdownCalls := 0
	manager.OnStart(func(context.Context) error {
		close(startEntered)
		<-releaseStart
		return nil
	})
	manager.OnShutdown(func(context.Context) error {
		shutdownCalls++
		return nil
	})

	startDone := make(chan error, 1)
	go func() { startDone <- manager.BeginStart(context.Background()) }()
	<-startEntered
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := manager.BeginShutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown error = %v, want deadline exceeded", err)
	}
	if shutdownCalls != 0 {
		t.Fatalf("shutdown hooks ran %d times before startup finished", shutdownCalls)
	}

	close(releaseStart)
	if err := <-startDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("start error = %v, want context.Canceled", err)
	}
	if err := manager.BeginShutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if shutdownCalls != 1 {
		t.Fatalf("shutdown hooks ran %d times, want 1", shutdownCalls)
	}
}
