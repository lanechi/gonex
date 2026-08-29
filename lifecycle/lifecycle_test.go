package lifecycle

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestMarkStartedRetriesAfterFailure(t *testing.T) {
	manager := New()
	var calls atomic.Int32
	manager.OnStarted(func(context.Context) error {
		if calls.Add(1) == 1 {
			return errors.New("not ready")
		}
		return nil
	})
	if err := manager.MarkStarted(context.Background()); err == nil {
		t.Fatal("first MarkStarted succeeded")
	}
	if err := manager.MarkStarted(context.Background()); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("hook calls = %d, want 2", calls.Load())
	}
}

func TestMarkStartedConcurrentCallReturnsPhaseInProgress(t *testing.T) {
	manager := New()
	var calls atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	manager.OnStarted(func(context.Context) error {
		calls.Add(1)
		close(entered)
		<-release
		return nil
	})

	first := make(chan error, 1)
	go func() { first <- manager.MarkStarted(context.Background()) }()
	<-entered
	if err := manager.MarkStarted(context.Background()); !errors.Is(err, ErrPhaseInProgress) {
		t.Fatalf("concurrent MarkStarted error = %v, want ErrPhaseInProgress", err)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("hook calls = %d, want 1", calls.Load())
	}
	if err := manager.MarkStarted(context.Background()); err != nil {
		t.Fatalf("completed MarkStarted was not idempotent: %v", err)
	}
}

func TestBeginStartRetriesAfterFailure(t *testing.T) {
	manager := New()
	var calls atomic.Int32
	manager.OnStart(func(context.Context) error {
		if calls.Add(1) == 1 {
			return errors.New("not ready")
		}
		return nil
	})
	if err := manager.BeginStart(context.Background()); err == nil {
		t.Fatal("first BeginStart succeeded")
	}
	if err := manager.BeginStart(context.Background()); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("hook calls = %d, want 2", calls.Load())
	}
}

func TestBeginStartConcurrentCallReturnsPhaseInProgress(t *testing.T) {
	manager := New()
	var calls atomic.Int32
	entered := make(chan struct{})
	release := make(chan struct{})
	manager.OnStart(func(context.Context) error {
		calls.Add(1)
		close(entered)
		<-release
		return nil
	})

	first := make(chan error, 1)
	go func() { first <- manager.BeginStart(context.Background()) }()
	<-entered
	if err := manager.BeginStart(context.Background()); !errors.Is(err, ErrPhaseInProgress) {
		t.Fatalf("concurrent BeginStart error = %v, want ErrPhaseInProgress", err)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("hook calls = %d, want 1", calls.Load())
	}
}

func TestMarkStartedDoesNotRunAfterShutdown(t *testing.T) {
	manager := New()
	var calls atomic.Int32
	manager.OnStarted(func(context.Context) error {
		calls.Add(1)
		return nil
	})
	if err := manager.BeginShutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := manager.MarkStarted(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("MarkStarted error = %v, want context.Canceled", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("OnStarted calls = %d, want 0", calls.Load())
	}
}
