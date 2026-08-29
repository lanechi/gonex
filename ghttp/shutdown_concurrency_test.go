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

func (*blockingShutdownScheduler) Start(context.Context) error       { return nil }
func (*blockingShutdownScheduler) Stop()                             {}
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

func TestConcurrentShutdownsShareResourceCleanupAttempt(t *testing.T) {
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
	go func() { first <- server.Shutdown(context.Background()) }()
	<-manager.waitEntered
	go func() { second <- server.Shutdown(context.Background()) }()
	close(manager.releaseWait)

	if err := <-first; err != nil {
		t.Fatalf("first shutdown returned %v", err)
	}
	if err := <-second; err != nil {
		t.Fatalf("second shutdown returned %v", err)
	}
}
