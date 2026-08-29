package ghttp

import (
	"context"
	"testing"

	"github.com/lanechi/gonex/logging"
	"github.com/lanechi/gonex/scheduler"
)

type blockingShutdownScheduler struct {
	waitEntered chan struct{}
	releaseWait chan struct{}
}

func (*blockingShutdownScheduler) Start(context.Context) error { return nil }
func (*blockingShutdownScheduler) Stop()                       {}
func (s *blockingShutdownScheduler) Wait(context.Context) error {
	select {
	case <-s.waitEntered:
	default:
		close(s.waitEntered)
	}
	<-s.releaseWait
	return nil
}
func (*blockingShutdownScheduler) Add(scheduler.Job) error           { return nil }
func (*blockingShutdownScheduler) Remove(string) error               { return nil }
func (*blockingShutdownScheduler) Jobs() []scheduler.JobInfo         { return nil }
func (*blockingShutdownScheduler) Use(...scheduler.Middleware) error { return nil }

func TestConcurrentShutdownsCompleteSuccessfully(t *testing.T) {
	manager := &blockingShutdownScheduler{
		waitEntered: make(chan struct{}),
		releaseWait: make(chan struct{}),
	}
	server := NewServer(
		WithLogger(logging.NewNopLogger()),
		WithScheduler(manager),
	)

	first := make(chan error, 1)
	second := make(chan error, 1)
	secondStarted := make(chan struct{})
	go func() { first <- server.Shutdown(context.Background()) }()
	<-manager.waitEntered
	go func() {
		close(secondStarted)
		second <- server.Shutdown(context.Background())
	}()
	<-secondStarted
	close(manager.releaseWait)

	if err := <-first; err != nil {
		t.Fatalf("first shutdown returned %v", err)
	}
	if err := <-second; err != nil {
		t.Fatalf("second shutdown returned %v", err)
	}
}

func TestShutdownCleanupAttemptsCoalesceOnlyWhileActive(t *testing.T) {
	server := NewServer(WithLogger(logging.NewNopLogger()))

	first, leader := server.beginShutdownCleanupAttempt()
	if !leader {
		t.Fatal("first cleanup attempt was not elected leader")
	}
	follower, leader := server.beginShutdownCleanupAttempt()
	if leader {
		t.Fatal("concurrent cleanup attempt unexpectedly became leader")
	}
	if follower != first {
		t.Fatal("concurrent cleanup attempt did not join active attempt")
	}

	server.finishShutdownCleanupAttempt(first, nil)
	select {
	case <-first.done:
	default:
		t.Fatal("completed cleanup attempt did not publish completion")
	}

	retry, leader := server.beginShutdownCleanupAttempt()
	if !leader {
		t.Fatal("sequential cleanup retry did not become leader")
	}
	if retry == first {
		t.Fatal("sequential cleanup retry reused completed attempt")
	}
	server.finishShutdownCleanupAttempt(retry, nil)
}
