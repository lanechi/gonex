package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"
)

type memoryJobStore struct {
	mu   sync.Mutex
	jobs []JobDefinition
}

func (store *memoryJobStore) ListEnabled(context.Context) ([]JobDefinition, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return append([]JobDefinition(nil), store.jobs...), nil
}
func (store *memoryJobStore) Get(_ context.Context, id string) (JobDefinition, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	for _, job := range store.jobs {
		if job.ID == id {
			return job, nil
		}
	}
	return JobDefinition{}, context.Canceled
}

func TestLoaderSynchronizesVersionedDefinitions(t *testing.T) {
	manager, err := New()
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryJobStore{jobs: []JobDefinition{{ID: "1", Name: "sync", Handler: "sync", Schedule: Every{Duration: time.Hour}, Version: 1, Enabled: true}}}
	registry := NewHandlerRegistry()
	called := make(chan struct{}, 1)
	if err := registry.Register("sync", func(context.Context, Execution) error { called <- struct{}{}; return nil }); err != nil {
		t.Fatal(err)
	}
	loader, err := NewLoader(store, registry, manager)
	if err != nil {
		t.Fatal(err)
	}
	if err := loader.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := manager.Jobs(); len(got) != 1 || got[0].Name != "sync" {
		t.Fatalf("jobs = %#v", got)
	}
	if err := loader.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := manager.Jobs(); len(got) != 1 {
		t.Fatalf("unchanged sync duplicated jobs: %#v", got)
	}
	store.jobs[0].Version = 2
	if err := loader.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := manager.Jobs(); len(got) != 1 {
		t.Fatalf("changed sync did not replace job: %#v", got)
	}
	manager.Stop()
}

func TestHandlerRegistryRejectsDuplicateNames(t *testing.T) {
	registry := NewHandlerRegistry()
	handler := func(context.Context, Execution) error { return nil }
	if err := registry.Register("job", handler); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("job", handler); err == nil {
		t.Fatal("duplicate handler name was accepted")
	}
	if _, ok := registry.Get("missing"); ok {
		t.Fatal("missing handler was found")
	}
}
