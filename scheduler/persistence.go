package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
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
	List(context.Context) ([]JobDefinition, error)
}

// Loader synchronizes enabled definitions into a runtime Scheduler.
type Loader struct {
	store      Store
	registry   HandlerRegistry
	scheduler  Scheduler
	mu         sync.Mutex
	loaded     map[string]loadedJob
	locker     Locker
	recorder   RunRecorder
	instanceID string
}

type loadedJob struct {
	Version    int64
	Name       string
	Definition JobDefinition
}

// LoaderOption configures optional persistent execution behavior.
type LoaderOption func(*Loader)

// WithLocker enables Singleton execution locking.
func WithLocker(locker Locker) LoaderOption { return func(loader *Loader) { loader.locker = locker } }

// WithRunRecorder records persistent executions.
func WithRunRecorder(recorder RunRecorder) LoaderOption {
	return func(loader *Loader) { loader.recorder = recorder }
}

// WithInstanceID identifies this application instance in run records.
func WithInstanceID(id string) LoaderOption { return func(loader *Loader) { loader.instanceID = id } }

// NewLoader creates a storage-neutral persistent job loader.
func NewLoader(store Store, registry HandlerRegistry, scheduler Scheduler, options ...LoaderOption) (*Loader, error) {
	if isNilValue(store) || isNilValue(registry) || isNilValue(scheduler) {
		return nil, fmt.Errorf("store, handler registry, and scheduler are required")
	}
	loader := &Loader{store: store, registry: registry, scheduler: scheduler, loaded: make(map[string]loadedJob)}
	for _, option := range options {
		if option != nil {
			option(loader)
		}
	}
	return loader, nil
}

// Sync loads enabled definitions and adds changed jobs, removing definitions no longer returned.
func (loader *Loader) Sync(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	definitions, err := loader.store.List(ctx)
	if err != nil {
		return err
	}
	loader.mu.Lock()
	defer loader.mu.Unlock()
	previousLoaded := make(map[string]loadedJob, len(loader.loaded))
	for id, job := range loader.loaded {
		previousLoaded[id] = job
	}
	seen := make(map[string]struct{}, len(definitions))
	desiredIDs := make(map[string]struct{}, len(definitions))
	desiredNames := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		if definition.ID == "" || definition.Name == "" {
			return fmt.Errorf("persistent job ID and name are required")
		}
		if _, exists := desiredIDs[definition.ID]; exists {
			return fmt.Errorf("duplicate persistent job ID %q", definition.ID)
		}
		desiredIDs[definition.ID] = struct{}{}
		if _, exists := desiredNames[definition.Name]; exists {
			return fmt.Errorf("duplicate persistent job name %q", definition.Name)
		}
		desiredNames[definition.Name] = struct{}{}
		if definition.Enabled {
			if definition.ExecutionMode == Singleton && isNilValue(loader.locker) {
				return fmt.Errorf("persistent job %q requires a Locker for Singleton execution", definition.Name)
			}
			if _, ok := loader.registry.Get(definition.Handler); !ok {
				return fmt.Errorf("persistent handler %q is not registered", definition.Handler)
			}
		}
	}
	// Remove stale and replaced records before adding desired records. This also
	// handles an ID change that reuses an existing runtime name.
	removed := make([]loadedJob, 0)
	for id, previous := range loader.loaded {
		current, exists := findDefinition(definitions, id)
		if !exists || !current.Enabled || current.Version != previous.Version || current.Name != previous.Name {
			if err := removePersistentJob(loader.scheduler, previous.Name); err != nil {
				return err
			}
			removed = append(removed, previous)
			delete(loader.loaded, id)
		}
	}
	for _, definition := range definitions {
		seen[definition.ID] = struct{}{}
		previous, exists := loader.loaded[definition.ID]
		if !definition.Enabled {
			continue
		}
		if exists && previous.Version == definition.Version && previous.Name == definition.Name {
			continue
		}
		handler, ok := loader.registry.Get(definition.Handler)
		if !ok {
			return fmt.Errorf("persistent handler %q is not registered", definition.Handler)
		}
		definitionCopy := definition
		definitionCopy.Payload = append(json.RawMessage(nil), definition.Payload...)
		if err := loader.scheduler.Add(Job{Name: definition.Name, Schedule: definition.Schedule, Timeout: definition.Timeout, OverlapPolicy: definition.OverlapPolicy, Handler: func(jobContext context.Context) error {
			return loader.execute(jobContext, handler, definitionCopy)
		}}); err != nil {
			loader.rollback(previousLoaded, removed)
			return err
		}
		loader.loaded[definition.ID] = loadedJob{Version: definition.Version, Name: definition.Name, Definition: definitionCopy}
	}
	for id, previous := range loader.loaded {
		if _, exists := seen[id]; !exists {
			if err := removePersistentJob(loader.scheduler, previous.Name); err != nil {
				loader.rollback(previousLoaded, removed)
				return err
			}
			removed = append(removed, previous)
			delete(loader.loaded, id)
		}
	}
	return nil
}

func (loader *Loader) rollback(previous map[string]loadedJob, removed []loadedJob) {
	for id, current := range loader.loaded {
		if old, exists := previous[id]; !exists || old.Name != current.Name || old.Version != current.Version {
			_ = loader.scheduler.Remove(current.Name)
		}
	}
	for id, old := range previous {
		if _, exists := loader.loaded[id]; exists {
			continue
		}
		if old.Definition.Enabled {
			if err := loader.addDefinition(old.Definition); err == nil {
				loader.loaded[id] = old
			}
		}
	}
}

func (loader *Loader) addDefinition(definition JobDefinition) error {
	handler, ok := loader.registry.Get(definition.Handler)
	if !ok {
		return fmt.Errorf("persistent handler %q is not registered", definition.Handler)
	}
	definition.Payload = append(json.RawMessage(nil), definition.Payload...)
	return loader.scheduler.Add(Job{Name: definition.Name, Schedule: definition.Schedule, Timeout: definition.Timeout, OverlapPolicy: definition.OverlapPolicy, Handler: func(ctx context.Context) error { return loader.execute(ctx, handler, definition) }})
}

func findDefinition(definitions []JobDefinition, id string) (JobDefinition, bool) {
	for _, definition := range definitions {
		if definition.ID == id {
			return definition, true
		}
	}
	return JobDefinition{}, false
}
func removePersistentJob(scheduler Scheduler, name string) error {
	if err := scheduler.Remove(name); err != nil && !errors.Is(err, ErrJobNotFound) {
		return err
	}
	return nil
}

// Run performs an initial synchronization and then polls until ctx is canceled.
func (loader *Loader) Run(ctx context.Context, interval time.Duration) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if interval <= 0 {
		return fmt.Errorf("loader polling interval must be positive")
	}
	if err := loader.Sync(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := loader.Sync(ctx); err != nil {
				return err
			}
		}
	}
}

func (loader *Loader) execute(ctx context.Context, handler PersistentHandler, definition JobDefinition) error {
	if definition.ExecutionMode == Singleton && loader.locker != nil {
		lock, acquired, err := loader.locker.TryLock(ctx, definition.ID, definition.Timeout)
		if err != nil {
			return err
		}
		if !acquired {
			return nil
		}
		if isNilValue(lock) {
			return fmt.Errorf("persistent locker acquired job %q without a lock", definition.ID)
		}
		defer func() { _ = lock.Unlock(context.Background()) }()
	}
	payload := append(json.RawMessage(nil), definition.Payload...)
	record := RunRecord{RunID: newRunID(), JobID: definition.ID, InstanceID: loader.instanceID, ScheduledAt: time.Now(), StartedAt: time.Now()}
	if loader.recorder != nil {
		_ = loader.recorder.Start(ctx, record)
	}
	err := handler(ctx, Execution{Definition: definition, Payload: payload})
	record.FinishedAt, record.Duration = time.Now(), time.Since(record.StartedAt)
	if err == nil {
		record.Status = RunSuccess
	} else if errors.Is(err, context.DeadlineExceeded) {
		record.Status = RunTimeout
	} else {
		record.Status, record.Error = RunFailed, err.Error()
	}
	if loader.recorder != nil {
		_ = loader.recorder.Finish(ctx, record)
	}
	return err
}

func isNilValue(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return v.IsNil()
	}
	return false
}
func newRunID() string { return fmt.Sprintf("run-%d", time.Now().UnixNano()) }

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
	RunID       string
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
