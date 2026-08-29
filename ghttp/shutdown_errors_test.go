package ghttp

import (
	"context"
	"errors"
	"testing"

	"github.com/lanechi/gonex/scheduler"
)

type shutdownErrorScheduler struct{ waitErr error }

func (*shutdownErrorScheduler) Start(context.Context) error         { return nil }
func (*shutdownErrorScheduler) Stop()                               {}
func (s *shutdownErrorScheduler) Wait(context.Context) error        { return s.waitErr }
func (*shutdownErrorScheduler) Add(scheduler.Job) error             { return nil }
func (*shutdownErrorScheduler) Remove(string) error                 { return nil }
func (*shutdownErrorScheduler) Jobs() []scheduler.JobInfo           { return nil }
func (*shutdownErrorScheduler) Use(...scheduler.Middleware) error   { return nil }

func TestShutdownJoinsIndependentCleanupFailures(t *testing.T) {
	lifecycleErr := errors.New("shutdown hook failed")
	schedulerErr := errors.New("scheduler wait failed")
	server := NewServer()
	server.lifecycle.OnShutdown(func(context.Context) error { return lifecycleErr })
	server.scheduler = &shutdownErrorScheduler{waitErr: schedulerErr}

	err := server.Shutdown(context.Background())
	if !errors.Is(err, lifecycleErr) {
		t.Fatalf("Shutdown error %v does not preserve lifecycle failure", err)
	}
	if !errors.Is(err, schedulerErr) {
		t.Fatalf("Shutdown error %v does not preserve scheduler failure", err)
	}
}
