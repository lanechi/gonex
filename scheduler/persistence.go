package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ExecutionMode controls whether a persistent job runs on one or every instance.
type ExecutionMode uint8

const (
	Singleton ExecutionMode = iota
	EveryInstance
)

// JobDefinition is the storage-neutral definition of a persistent job.
// Handler contains a stable application registration name, not a Go function path.
type JobDefinition struct {
	ID            string
	Name          string
	Handler       string
	Schedule      Schedule
	Payload       json.RawMessage
	Enabled       bool
	Version       int64
	Timeout       time.Duration
	OverlapPolicy OverlapPolicy
	ExecutionMode ExecutionMode
}

// Execution is the input passed to a persistent handler.
type Execution struct {
	Definition JobDefinition
	Payload    json.RawMessage
}

// PersistentHandler executes a persistent job definition.
type PersistentHandler func(context.Context, Execution) error

// HandlerRegistry resolves stable application handler names explicitly.
type HandlerRegistry interface {
	Register(name string, handler PersistentHandler) error
	Get(name string) (PersistentHandler, bool)
}

type handlerRegistry struct {
	mu       sync.RWMutex
	handlers map[string]PersistentHandler
}

// NewHandlerRegistry creates an empty explicit handler registry.
func NewHandlerRegistry() HandlerRegistry {
	return &handlerRegistry{handlers: make(map[string]PersistentHandler)}
}

func (registry *handlerRegistry) Register(name string, handler PersistentHandler) error {
	if name == "" || handler == nil {
		return fmt.Errorf("persistent handler name and function are required")
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.handlers[name]; exists {
		return fmt.Errorf("persistent handler %q is already registered", name)
	}
	registry.handlers[name] = handler
	return nil
}

func (registry *handlerRegistry) Get(name string) (PersistentHandler, bool) {
	registry.mu.RLock()
	handler, ok := registry.handlers[name]
	registry.mu.RUnlock()
	return handler, ok
}

// Store provides persistent definitions without coupling Scheduler to a database.
type Store interface {
	ListEnabled(context.Context) ([]JobDefinition, error)
	Get(context.Context, string) (JobDefinition, error)
}

// Loader synchronizes enabled definitions into a runtime Scheduler.
type Loader struct {
	store     Store
	registry  HandlerRegistry
	scheduler Scheduler
	mu        sync.Mutex
	loaded    map[string]loadedJob
}

type loadedJob struct {
	Version int64
	Name    string
}

// NewLoader creates a storage-neutral persistent job loader.
func NewLoader(store Store, registry HandlerRegistry, scheduler Scheduler) (*Loader, error) {
	if store == nil || registry == nil || scheduler == nil {
		return nil, fmt.Errorf("store, handler registry, and scheduler are required")
	}
	return &Loader{store: store, registry: registry, scheduler: scheduler, loaded: make(map[string]loadedJob)}, nil
}

// Sync loads enabled definitions and adds changed jobs, removing definitions no longer returned.
func (loader *Loader) Sync(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	definitions, err := loader.store.ListEnabled(ctx)
	if err != nil {
		return err
	}
	loader.mu.Lock()
	defer loader.mu.Unlock()
	seen := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		if definition.ID == "" || definition.Name == "" {
			return fmt.Errorf("persistent job ID and name are required")
		}
		seen[definition.ID] = struct{}{}
		previous, exists := loader.loaded[definition.ID]
		if exists && previous.Version == definition.Version {
			continue
		}
		if exists {
			_ = loader.scheduler.Remove(previous.Name)
		}
		handler, ok := loader.registry.Get(definition.Handler)
		if !ok {
			return fmt.Errorf("persistent handler %q is not registered", definition.Handler)
		}
		definitionCopy := definition
		if err := loader.scheduler.Add(Job{Name: definition.Name, Schedule: definition.Schedule, Timeout: definition.Timeout, OverlapPolicy: definition.OverlapPolicy, Handler: func(jobContext context.Context) error {
			return handler(jobContext, Execution{Definition: definitionCopy, Payload: definitionCopy.Payload})
		}}); err != nil {
			return err
		}
		loader.loaded[definition.ID] = loadedJob{Version: definition.Version, Name: definition.Name}
	}
	for id, previous := range loader.loaded {
		if _, exists := seen[id]; !exists {
			_ = loader.scheduler.Remove(previous.Name)
			delete(loader.loaded, id)
		}
	}
	return nil
}

// Lock represents an acquired distributed lock.
type Lock interface{ Unlock(context.Context) error }

// Locker provides an optional distributed execution lock without naming a backend.
type Locker interface {
	TryLock(context.Context, string, time.Duration) (Lock, bool, error)
}

// RunStatus describes the outcome of one persistent execution.
type RunStatus string

const (
	RunSuccess RunStatus = "success"
	RunFailed  RunStatus = "failed"
	RunSkipped RunStatus = "skipped"
	RunTimeout RunStatus = "timeout"
)

// RunRecord is a storage-neutral execution history record.
type RunRecord struct {
	JobID       string
	InstanceID  string
	ScheduledAt time.Time
	StartedAt   time.Time
	FinishedAt  time.Time
	Status      RunStatus
	Error       string
	Duration    time.Duration
}

// RunRecorder records persistent execution start and completion.
type RunRecorder interface {
	Start(context.Context, RunRecord) error
	Finish(context.Context, RunRecord) error
}
