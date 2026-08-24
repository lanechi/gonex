package lifecycle

import (
	"context"
	"errors"
	"sync"
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

func TestMarkStartedConcurrentCallsRunHooksOnce(t *testing.T) {
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

	const callers = 8
	var group sync.WaitGroup
	errors := make(chan error, callers)
	for range callers {
		group.Add(1)
		go func() {
			defer group.Done()
			errors <- manager.MarkStarted(context.Background())
		}()
	}
	<-entered
	close(release)
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("hook calls = %d, want 1", calls.Load())
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

func TestBeginStartConcurrentCallsRunHooksOnce(t *testing.T) {
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

	const callers = 8
	var group sync.WaitGroup
	errors := make(chan error, callers)
	for range callers {
		group.Add(1)
		go func() {
			defer group.Done()
			errors <- manager.BeginStart(context.Background())
		}()
	}
	<-entered
	close(release)
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
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

func TestStartupPhaseWaitersReadTheirOwnResult(t *testing.T) {
	manager := New()
	beginErr := errors.New("begin failed")
	startedErr := errors.New("started failed")

	beginAttempt := &phaseAttempt{done: make(chan struct{}), err: beginErr}
	manager.startRunning = true
	manager.startAttempt = beginAttempt
	beginResult := make(chan error, 1)
	go func() {
		beginResult <- manager.BeginStart(context.Background())
	}()
	close(beginAttempt.done)
	if err := <-beginResult; !errors.Is(err, beginErr) {
		t.Fatalf("BeginStart waiter error = %v, want %v", err, beginErr)
	}

	manager.startRunning = false
	manager.startHooksRun = true
	startedAttempt := &phaseAttempt{done: make(chan struct{}), err: startedErr}
	manager.starting = true
	manager.startedAttempt = startedAttempt
	startedResult := make(chan error, 1)
	go func() {
		startedResult <- manager.MarkStarted(context.Background())
	}()
	close(startedAttempt.done)
	if err := <-startedResult; !errors.Is(err, startedErr) {
		t.Fatalf("MarkStarted waiter error = %v, want %v", err, startedErr)
	}
}
