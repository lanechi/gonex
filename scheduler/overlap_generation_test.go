package scheduler

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestQueueOneRunsQueuedReplacementHandler(t *testing.T) {
	gate := &overlapGate{policy: QueueOne}
	oldToken := &struct{ generation int }{generation: 1}
	newToken := &struct{ generation int }{generation: 2}
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	newRan := make(chan struct{}, 1)
	var oldRuns atomic.Int32

	go func() {
		defer close(finished)
		gate.runWithToken(oldToken, func() {
			oldRuns.Add(1)
			close(entered)
			<-release
		})
	}()
	<-entered

	executed, queued := gate.runWithToken(newToken, func() { newRan <- struct{}{} })
	if executed || !queued {
		t.Fatalf("replacement trigger executed=%t queued=%t, want false/true", executed, queued)
	}
	close(release)

	select {
	case <-newRan:
	case <-time.After(time.Second):
		t.Fatal("queued replacement handler did not run")
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("active overlap run did not finish")
	}
	if got := oldRuns.Load(); got != 1 {
		t.Fatalf("old handler ran %d times, want 1", got)
	}
}
