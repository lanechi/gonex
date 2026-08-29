package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestShutdownDuringActiveStartRecordsIntentWithoutWaiting(t *testing.T) {
	manager := New()
	startEntered := make(chan struct{})
	releaseStart := make(chan struct{})
	shutdownRan := make(chan struct{}, 1)
	manager.OnStart(func(context.Context) error {
		close(startEntered)
		<-releaseStart
		return nil
	})
	manager.OnShutdown(func(context.Context) error {
		shutdownRan <- struct{}{}
		return nil
	})

	startDone := make(chan error, 1)
	go func() { startDone <- manager.BeginStart(context.Background()) }()
	<-startEntered

	started := time.Now()
	if err := manager.BeginShutdown(context.Background()); !errors.Is(err, ErrStartupInProgress) {
		t.Fatalf("shutdown error = %v, want ErrStartupInProgress", err)
	}
	if time.Since(started) > 100*time.Millisecond {
		t.Fatal("shutdown waited for active startup instead of returning the deferred transition error")
	}
	select {
	case <-shutdownRan:
		t.Fatal("shutdown hook ran while start hook was active")
	default:
	}

	close(releaseStart)
	if err := <-startDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("start error = %v, want context.Canceled", err)
	}
	if err := manager.BeginShutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-shutdownRan:
	default:
		t.Fatal("shutdown hook did not run after startup unwound")
	}
}

func TestOnStartCanRequestShutdownWithoutDeadlock(t *testing.T) {
	manager := New()
	requestResult := make(chan error, 1)
	manager.OnStart(func(ctx context.Context) error {
		requestResult <- manager.BeginShutdown(ctx)
		return nil
	})

	if err := manager.BeginStart(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("BeginStart error = %v, want context.Canceled", err)
	}
	if err := <-requestResult; !errors.Is(err, ErrStartupInProgress) {
		t.Fatalf("reentrant shutdown error = %v, want ErrStartupInProgress", err)
	}
	if err := manager.BeginShutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestOnShutdownCanCallStopWithoutDeadlock(t *testing.T) {
	manager := New()
	reentrant := make(chan error, 1)
	manager.OnShutdown(func(ctx context.Context) error {
		reentrant <- manager.Stop(ctx)
		return nil
	})
	if err := manager.BeginShutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-reentrant; !errors.Is(err, ErrPhaseInProgress) {
		t.Fatalf("reentrant Stop error = %v, want ErrPhaseInProgress", err)
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestOnStopCanCallStopWithoutDeadlock(t *testing.T) {
	manager := New()
	reentrant := make(chan error, 1)
	manager.OnStop(func(ctx context.Context) error {
		reentrant <- manager.Stop(ctx)
		return nil
	})
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-reentrant; !errors.Is(err, ErrPhaseInProgress) {
		t.Fatalf("reentrant Stop error = %v, want ErrPhaseInProgress", err)
	}
}
