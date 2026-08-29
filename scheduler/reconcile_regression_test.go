package scheduler

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

type faultScheduler struct {
	jobs       map[string]Job
	failAdd    string
	failRemove string
}

func newFaultScheduler() *faultScheduler { return &faultScheduler{jobs: make(map[string]Job)} }
func (*faultScheduler) Start(context.Context) error { return nil }
func (*faultScheduler) Stop()                       {}
func (*faultScheduler) Wait(context.Context) error  { return nil }
func (*faultScheduler) Use(...Middleware) error     { return nil }
func (scheduler *faultScheduler) Add(job Job) error {
	if job.Name == scheduler.failAdd {
		return fmt.Errorf("forced add failure: %s", job.Name)
	}
	if _, exists := scheduler.jobs[job.Name]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateJob, job.Name)
	}
	scheduler.jobs[job.Name] = job
	return nil
}
func (scheduler *faultScheduler) Remove(name string) error {
	if name == scheduler.failRemove {
		return fmt.Errorf("forced remove failure: %s", name)
	}
	if _, exists := scheduler.jobs[name]; !exists {
		return fmt.Errorf("%w: %s", ErrJobNotFound, name)
	}
	delete(scheduler.jobs, name)
	return nil
}
func (scheduler *faultScheduler) Jobs() []JobInfo {
	result := make([]JobInfo, 0, len(scheduler.jobs))
	for name, job := range scheduler.jobs {
		result = append(result, JobInfo{Name: name, Schedule: job.Schedule})
	}
	return result
}

func persistentDefinition(id, name string, version int64) JobDefinition {
	return JobDefinition{ID: id, Name: name, Handler: "run", Schedule: Every{Duration: time.Hour}, Enabled: true, Version: version, ExecutionMode: EveryInstance}
}

func persistentRegistry(t *testing.T) HandlerRegistry {
	t.Helper()
	registry := NewHandlerRegistry()
	if err := registry.Register("run", func(context.Context, Execution) error { return nil }); err != nil {
		t.Fatal(err)
	}
	return registry
}

func assertRuntimeNames(t *testing.T, runtime *faultScheduler, names ...string) {
	t.Helper()
	if len(runtime.jobs) != len(names) {
		t.Fatalf("runtime jobs = %#v, want %v", runtime.jobs, names)
	}
	for _, name := range names {
		if _, exists := runtime.jobs[name]; !exists {
			t.Fatalf("runtime job %q missing: %#v", name, runtime.jobs)
		}
	}
}

func TestLoaderRollbackRestoresStateAfterRemoveFailure(t *testing.T) {
	store := &memoryJobStore{jobs: []JobDefinition{persistentDefinition("1", "a", 1), persistentDefinition("2", "b", 1)}}
	runtime := newFaultScheduler()
	loader, err := NewLoader(store, persistentRegistry(t), runtime)
	if err != nil { t.Fatal(err) }
	if err := loader.Sync(context.Background()); err != nil { t.Fatal(err) }
	store.jobs = nil
	runtime.failRemove = "b"
	if err := loader.Sync(context.Background()); err == nil { t.Fatal("remove failure was ignored") }
	assertRuntimeNames(t, runtime, "a", "b")
	if len(loader.loaded) != 2 { t.Fatalf("loaded = %#v", loader.loaded) }
}

func TestLoaderRollbackRestoresStateAfterAddFailure(t *testing.T) {
	store := &memoryJobStore{jobs: []JobDefinition{persistentDefinition("1", "a", 1), persistentDefinition("2", "b", 1)}}
	runtime := newFaultScheduler()
	loader, err := NewLoader(store, persistentRegistry(t), runtime)
	if err != nil { t.Fatal(err) }
	if err := loader.Sync(context.Background()); err != nil { t.Fatal(err) }
	store.jobs = []JobDefinition{persistentDefinition("1", "a2", 2), persistentDefinition("2", "b2", 2)}
	runtime.failAdd = "b2"
	if err := loader.Sync(context.Background()); err == nil { t.Fatal("add failure was ignored") }
	assertRuntimeNames(t, runtime, "a", "b")
	if loader.loaded["1"].Name != "a" || loader.loaded["2"].Name != "b" { t.Fatalf("loaded = %#v", loader.loaded) }
}

func TestLoaderPrevalidatesBeforeRemovingOldVersion(t *testing.T) {
	store := &memoryJobStore{jobs: []JobDefinition{persistentDefinition("1", "job", 1)}}
	runtime, _ := New()
	loader, err := NewLoader(store, persistentRegistry(t), runtime)
	if err != nil { t.Fatal(err) }
	if err := loader.Sync(context.Background()); err != nil { t.Fatal(err) }
	bad := persistentDefinition("1", "job", 2)
	bad.Schedule = Cron{Expr: "not a cron"}
	store.jobs = []JobDefinition{bad}
	if err := loader.Sync(context.Background()); err == nil { t.Fatal("invalid cron was accepted") }
	jobs := runtime.Jobs()
	if len(jobs) != 1 || jobs[0].Name != "job" { t.Fatalf("old runtime job was lost: %#v", jobs) }
	if loader.loaded["1"].Version != 1 { t.Fatalf("loaded version = %d", loader.loaded["1"].Version) }
}

func TestLoaderAllowsNewIDToReuseReleasedName(t *testing.T) {
	store := &memoryJobStore{jobs: []JobDefinition{persistentDefinition("old", "job", 1)}}
	runtime, _ := New()
	loader, err := NewLoader(store, persistentRegistry(t), runtime)
	if err != nil { t.Fatal(err) }
	if err := loader.Sync(context.Background()); err != nil { t.Fatal(err) }
	store.jobs = []JobDefinition{persistentDefinition("new", "job", 1)}
	if err := loader.Sync(context.Background()); err != nil { t.Fatal(err) }
	if _, exists := loader.loaded["old"]; exists { t.Fatal("old ID remained loaded") }
	if loader.loaded["new"].Name != "job" { t.Fatalf("loaded = %#v", loader.loaded) }
}

func TestLoaderRejectsInvalidExecutionMode(t *testing.T) {
	job := persistentDefinition("1", "job", 1)
	job.ExecutionMode = ExecutionMode(99)
	loader, err := NewLoader(&memoryJobStore{jobs: []JobDefinition{job}}, persistentRegistry(t), newTestScheduler())
	if err != nil { t.Fatal(err) }
	if err := loader.Sync(context.Background()); err == nil { t.Fatal("invalid execution mode was accepted") }
}

func TestRemovePersistentJobAcceptsAlreadyMissing(t *testing.T) {
	if err := removePersistentJob(newFaultScheduler(), "missing"); err != nil && !errors.Is(err, ErrJobNotFound) {
		t.Fatal(err)
	}
}
