package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestJobsConcurrentWithStartUsesImmutableSnapshot(t *testing.T) {
	runtime, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Add(Job{
		Name:     "snapshot",
		Schedule: Every{Duration: time.Hour},
		Handler:  func(context.Context) error { return nil },
	}); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		for index := 0; index < 500; index++ {
			jobs := runtime.Jobs()
			if len(jobs) != 1 || jobs[0].Name != "snapshot" {
				t.Errorf("jobs = %#v", jobs)
				return
			}
		}
	}()
	go func() {
		defer wait.Done()
		<-start
		if err := runtime.Start(context.Background()); err != nil {
			t.Errorf("Start() error = %v", err)
		}
	}()
	close(start)
	wait.Wait()
	runtime.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := runtime.Wait(ctx); err != nil {
		t.Fatal(err)
	}
}
