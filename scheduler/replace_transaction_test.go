package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/go-co-op/gocron/v2"
)

func TestFailedReplaceDoesNotRunPendingImmediateGeneration(t *testing.T) {
	configured, err := New()
	if err != nil {
		t.Fatal(err)
	}
	manager := configured.(*manager)
	if err := manager.Add(Job{
		Name:     "replace-transaction",
		Schedule: Every{Duration: time.Hour},
		Handler:  func(context.Context) error { return nil },
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		manager.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = manager.Wait(ctx)
	}()

	manager.mu.RLock()
	old := manager.jobs["replace-transaction"]
	inner := manager.inner
	manager.mu.RUnlock()
	if old == nil || old.inner == nil || inner == nil {
		t.Fatal("old engine job was not installed")
	}
	if err := inner.RemoveJob(old.inner.ID()); err != nil {
		t.Fatalf("remove old engine job to force replace failure: %v", err)
	}

	newRan := make(chan struct{}, 1)
	err = manager.Replace(Job{
		Name:           "replace-transaction",
		Schedule:       Every{Duration: time.Hour},
		RunImmediately: true,
		Handler: func(context.Context) error {
			newRan <- struct{}{}
			return nil
		},
	})
	if err == nil {
		t.Fatal("Replace succeeded after the old engine job had already disappeared")
	}
	select {
	case <-newRan:
		t.Fatal("uncommitted replacement handler executed")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestCleanupFailureLeavesPrimaryJobInstalled(t *testing.T) {
	configured, err := New()
	if err != nil {
		t.Fatal(err)
	}
	manager := configured.(*manager)
	if err := manager.Add(Job{
		Name:     "cleanup-order",
		Schedule: Every{Duration: time.Hour},
		Handler:  func(context.Context) error { return nil },
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		manager.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = manager.Wait(ctx)
	}()

	other, err := gocron.NewScheduler()
	if err != nil {
		t.Fatal(err)
	}
	defer other.Shutdown()
	orphan, err := other.NewJob(
		gocron.DurationJob(time.Hour),
		gocron.NewTask(func() {}),
	)
	if err != nil {
		t.Fatal(err)
	}

	manager.mu.Lock()
	old := manager.jobs["cleanup-order"]
	if old == nil || old.inner == nil {
		manager.mu.Unlock()
		t.Fatal("primary engine job was not installed")
	}
	primaryID := old.inner.ID()
	old.cleanup = append(old.cleanup, orphan)
	manager.mu.Unlock()

	err = manager.Replace(Job{
		Name:     "cleanup-order",
		Schedule: Every{Duration: 2 * time.Hour},
		Handler:  func(context.Context) error { return nil },
	})
	if err == nil {
		t.Fatal("Replace succeeded despite stale cleanup handle from another engine")
	}

	manager.mu.RLock()
	current := manager.jobs["cleanup-order"]
	primaryStillInstalled := current == old && old.inner != nil && old.inner.ID() == primaryID
	manager.mu.RUnlock()
	if !primaryStillInstalled {
		t.Fatal("cleanup failure removed the primary old engine job")
	}
}
