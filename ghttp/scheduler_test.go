package ghttp

import (
	"context"
	"testing"
	"time"

	"github.com/lanechi/gonex/logging"
	"github.com/lanechi/gonex/scheduler"
)

func TestServerOwnsIndependentSchedulers(t *testing.T) {
	first := NewServer()
	second := NewServer()
	if first.Scheduler() == second.Scheduler() {
		t.Fatal("servers unexpectedly share a scheduler")
	}
}

func TestDisabledServerSchedulerRemainsUsable(t *testing.T) {
	server := NewServer(WithSchedulerOptions(SchedulerOptions{Enabled: false}))
	if server.Scheduler() == nil {
		t.Fatal("disabled server returned a nil scheduler")
	}
	if err := server.Scheduler().Add(scheduler.Job{
		Name: "disabled-job", Schedule: scheduler.Every{Duration: time.Hour},
		Handler: func(context.Context) error { return nil },
	}); err != nil {
		t.Fatalf("Add() on disabled scheduler failed: %v", err)
	}
	if len(server.Scheduler().Jobs()) != 1 {
		t.Fatalf("Jobs() length = %d, want 1", len(server.Scheduler().Jobs()))
	}
}

func TestServerDocumentsInjectedSchedulerOwnership(t *testing.T) {
	manager, err := scheduler.New()
	if err != nil {
		t.Fatal(err)
	}
	first := NewServer(WithScheduler(manager))
	if err := first.Err(); err != nil {
		t.Fatal(err)
	}
	second := NewServer(WithScheduler(manager))
	if err := second.Err(); err != nil {
		t.Fatalf("injected scheduler was rejected without global ownership tracking: %v", err)
	}
}

func TestServerSchedulerLifecycleCancelsJobs(t *testing.T) {
	manager, err := scheduler.New()
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(
		WithLogger(logging.NewNopLogger()),
		WithScheduler(manager),
	)
	started := make(chan struct{})
	canceled := make(chan struct{})
	if err := manager.Add(scheduler.Job{
		Name:           "observe-shutdown",
		Schedule:       scheduler.Every{Duration: time.Hour},
		RunImmediately: true,
		Handler: func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			close(canceled)
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}

	runContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := server.lifecycle.BeginStart(runContext); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler job did not start with the server lifecycle")
	}
	cancel()
	shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	if err := server.lifecycle.BeginShutdown(shutdownContext); err != nil {
		t.Fatal(err)
	}
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("server shutdown did not cancel scheduler job context")
	}
	if err := server.lifecycle.Stop(shutdownContext); err != nil {
		t.Fatal(err)
	}
	if err := manager.Wait(shutdownContext); err != nil {
		t.Fatal(err)
	}
}

func TestServerSchedulerStopDoesNotBlockShutdownHooks(t *testing.T) {
	manager, err := scheduler.New()
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(WithLogger(logging.NewNopLogger()), WithScheduler(manager))
	started := make(chan struct{})
	release := make(chan struct{})
	if err := manager.Add(scheduler.Job{
		Name:           "draining-job",
		Schedule:       scheduler.Every{Duration: time.Hour},
		RunImmediately: true,
		Handler: func(context.Context) error {
			close(started)
			<-release
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.lifecycle.BeginStart(context.Background()); err != nil {
		t.Fatal(err)
	}
	<-started
	begin := time.Now()
	if err := server.lifecycle.BeginShutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(begin); elapsed > 100*time.Millisecond {
		t.Fatalf("OnShutdown waited for scheduler job: %s", elapsed)
	}
	close(release)
	if err := manager.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}
