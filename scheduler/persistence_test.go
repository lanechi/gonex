package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type testLock struct{ unlocked chan struct{} }

func (lock *testLock) Unlock(context.Context) error { close(lock.unlocked); return nil }

type testLocker struct {
	lock     *testLock
	acquired bool
}

func (locker *testLocker) TryLock(context.Context, string, time.Duration) (Lock, bool, error) {
	if !locker.acquired {
		return nil, false, nil
	}
	return locker.lock, true, nil
}

type testRecorder struct {
	mu      sync.Mutex
	records []RunRecord
}

func (recorder *testRecorder) Start(_ context.Context, record RunRecord) error {
	recorder.mu.Lock()
	recorder.records = append(recorder.records, record)
	recorder.mu.Unlock()
	return nil
}
func (recorder *testRecorder) Finish(_ context.Context, record RunRecord) error {
	recorder.mu.Lock()
	recorder.records = append(recorder.records, record)
	recorder.mu.Unlock()
	return nil
}

type memoryJobStore struct {
	mu   sync.Mutex
	jobs []JobDefinition
}

func (store *memoryJobStore) List(context.Context) ([]JobDefinition, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make([]JobDefinition, len(store.jobs))
	for index, job := range store.jobs {
		result[index] = cloneJobDefinition(job)
	}
	return result, nil
}

func TestLoaderSynchronizesVersionedDefinitions(t *testing.T) {
	manager, err := New()
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryJobStore{jobs: []JobDefinition{{ID: "1", Name: "sync", Handler: "sync", Schedule: Every{Duration: time.Hour}, Version: 1, Enabled: true, ExecutionMode: EveryInstance}}}
	registry := NewHandlerRegistry()
	if err := registry.Register("sync", func(context.Context, Execution) error { return nil }); err != nil {
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
	store.mu.Lock()
	store.jobs[0].Version = 2
	store.mu.Unlock()
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

func TestLoaderSingletonAndRecorder(t *testing.T) {
	manager, err := New()
	if err != nil {
		t.Fatal(err)
	}
	lock := &testLock{unlocked: make(chan struct{})}
	recorder := &testRecorder{}
	loader, err := NewLoader(&memoryJobStore{jobs: []JobDefinition{{ID: "1", Name: "single", Handler: "run", Schedule: Every{Duration: time.Hour}, Enabled: true, ExecutionMode: Singleton, Payload: []byte(`{"value":1}`)}}}, NewHandlerRegistry(), manager, WithLocker(&testLocker{lock: lock, acquired: true}), WithRunRecorder(recorder))
	if err != nil {
		t.Fatal(err)
	}
	if err := loader.registry.Register("run", func(ctx context.Context, execution Execution) error {
		execution.Definition.Payload = append(execution.Definition.Payload, 'x')
		return ctx.Err()
	}); err != nil {
		t.Fatal(err)
	}
	if err := loader.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	definition := loader.loaded["1"].Definition
	handler, ok := loader.registry.Get("run")
	if !ok {
		t.Fatal("handler was not registered")
	}
	if err := loader.execute(context.Background(), handler, definition); err != nil {
		t.Fatal(err)
	}
	select {
	case <-lock.unlocked:
	case <-time.After(time.Second):
		t.Fatal("singleton lock was not released")
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.records) != 2 || recorder.records[0].RunID == "" || recorder.records[0].RunID != recorder.records[1].RunID {
		t.Fatalf("records = %#v", recorder.records)
	}
	if recorder.records[0].Status != RunRunning || recorder.records[1].Status != RunSuccess {
		t.Fatalf("statuses = %q, %q", recorder.records[0].Status, recorder.records[1].Status)
	}
}

func TestLoaderRejectsSingletonWithoutLocker(t *testing.T) {
	registry := NewHandlerRegistry()
	_ = registry.Register("run", func(context.Context, Execution) error { return errors.New("unused") })
	loader, err := NewLoader(&memoryJobStore{jobs: []JobDefinition{{ID: "1", Name: "single", Handler: "run", Schedule: Every{Duration: time.Hour}, Enabled: true, ExecutionMode: Singleton}}}, registry, newTestScheduler())
	if err != nil {
		t.Fatal(err)
	}
	if err := loader.Sync(context.Background()); err == nil {
		t.Fatal("Singleton without Locker was accepted")
	}
}

type nilLockLocker struct{}

func (nilLockLocker) TryLock(context.Context, string, time.Duration) (Lock, bool, error) {
	var lock *testLock
	return lock, true, nil
}

func TestLoaderRejectsAcquiredNilLock(t *testing.T) {
	registry := NewHandlerRegistry()
	if err := registry.Register("run", func(context.Context, Execution) error { return nil }); err != nil {
		t.Fatal(err)
	}
	loader, err := NewLoader(&memoryJobStore{}, registry, newTestScheduler(), WithLocker(nilLockLocker{}))
	if err != nil {
		t.Fatal(err)
	}
	definition := JobDefinition{ID: "1", Name: "single", Handler: "run", ExecutionMode: Singleton}
	if err := loader.execute(context.Background(), func(context.Context, Execution) error { return nil }, definition); err == nil {
		t.Fatal("acquired nil lock was accepted")
	}
}

func TestLoaderRejectsDuplicateIDs(t *testing.T) {
	registry := NewHandlerRegistry()
	if err := registry.Register("run", func(context.Context, Execution) error { return nil }); err != nil {
		t.Fatal(err)
	}
	store := &memoryJobStore{jobs: []JobDefinition{
		{ID: "same", Name: "one", Handler: "run", Enabled: true},
		{ID: "same", Name: "two", Handler: "run", Enabled: true},
	}}
	loader, err := NewLoader(store, registry, newTestScheduler())
	if err != nil {
		t.Fatal(err)
	}
	if err := loader.Sync(context.Background()); err == nil {
		t.Fatal("duplicate persistent IDs were accepted")
	}
}

func newTestScheduler() MutableScheduler { manager, _ := New(); return manager }
