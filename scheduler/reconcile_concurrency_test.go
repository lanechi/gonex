package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"
)

type orderedSnapshotStore struct {
	mu            sync.Mutex
	calls         int
	firstStarted  chan struct{}
	secondStarted chan struct{}
	releaseFirst  chan struct{}
}

func (store *orderedSnapshotStore) List(context.Context) ([]JobDefinition, error) {
	store.mu.Lock()
	store.calls++
	call := store.calls
	store.mu.Unlock()
	switch call {
	case 1:
		close(store.firstStarted)
		<-store.releaseFirst
		return []JobDefinition{persistentDefinition("1", "job", 1)}, nil
	case 2:
		close(store.secondStarted)
		return []JobDefinition{persistentDefinition("1", "job", 2)}, nil
	default:
		return []JobDefinition{persistentDefinition("1", "job", 2)}, nil
	}
}

func TestLoaderSerializesStoreSnapshotWithReconciliation(t *testing.T) {
	store := &orderedSnapshotStore{
		firstStarted:  make(chan struct{}),
		secondStarted: make(chan struct{}),
		releaseFirst:  make(chan struct{}),
	}
	runtime, err := New()
	if err != nil {
		t.Fatal(err)
	}
	loader, err := NewLoader(store, persistentRegistry(t), runtime)
	if err != nil {
		t.Fatal(err)
	}

	firstDone := make(chan error, 1)
	go func() { firstDone <- loader.Sync(context.Background()) }()
	<-store.firstStarted

	secondDone := make(chan error, 1)
	go func() { secondDone <- loader.Sync(context.Background()) }()
	select {
	case <-store.secondStarted:
		t.Fatal("second Store.List started before the first Sync committed")
	case <-time.After(20 * time.Millisecond):
	}

	close(store.releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	if loader.loaded["1"].Version != 2 {
		t.Fatalf("loaded version = %d, want 2", loader.loaded["1"].Version)
	}
}
