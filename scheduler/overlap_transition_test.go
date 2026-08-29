package scheduler

import (
	"testing"
	"time"
)

func TestQueueOneTransitionFromAllowOverlapDrainsAcceptedQueue(t *testing.T) {
	gate := &overlapGate{policy: AllowOverlap}
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	finished := make(chan struct{}, 2)

	for index := 0; index < 2; index++ {
		go func(token int) {
			gate.runWithToken(token, func() {
				entered <- struct{}{}
				<-release
			})
			finished <- struct{}{}
		}(index)
	}
	for index := 0; index < 2; index++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("allow-overlap execution did not start")
		}
	}

	gate.setPolicy(QueueOne)
	queuedRan := make(chan struct{}, 1)
	executed, queued := gate.runWithToken("replacement", func() { queuedRan <- struct{}{} })
	if executed || !queued {
		t.Fatalf("replacement trigger executed=%t queued=%t, want false/true", executed, queued)
	}

	close(release)
	select {
	case <-queuedRan:
	case <-time.After(time.Second):
		t.Fatal("queued replacement did not run after allow-overlap executions drained")
	}
	for index := 0; index < 2; index++ {
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Fatal("allow-overlap execution did not finish")
		}
	}
	if gate.isRunning() {
		t.Fatal("overlap gate remained active after all work completed")
	}
}

func TestAcceptedQueueSurvivesPolicyChange(t *testing.T) {
	gate := &overlapGate{policy: QueueOne}
	entered := make(chan struct{})
	release := make(chan struct{})
	activeDone := make(chan struct{})
	go func() {
		gate.run(func() {
			close(entered)
			<-release
		})
		close(activeDone)
	}()
	<-entered

	queuedRan := make(chan struct{}, 1)
	executed, queued := gate.run(func() { queuedRan <- struct{}{} })
	if executed || !queued {
		t.Fatalf("queued trigger executed=%t queued=%t, want false/true", executed, queued)
	}
	gate.setPolicy(SkipIfRunning)
	close(release)

	select {
	case <-queuedRan:
	case <-time.After(time.Second):
		t.Fatal("accepted queued trigger was dropped by policy change")
	}
	select {
	case <-activeDone:
	case <-time.After(time.Second):
		t.Fatal("active execution did not finish")
	}
}
