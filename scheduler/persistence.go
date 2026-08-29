package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
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

// Execution is the immutable input passed to a persistent handler. Definition
// is cloned for each run, including Payload, so handler mutation cannot change
// loader state or later executions.
type Execution struct {
	Definition JobDefinition
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
	name = strings.TrimSpace(name)
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
	name = strings.TrimSpace(name)
	registry.mu.RLock()
	handler, ok := registry.handlers[name]
	registry.mu.RUnlock()
	return handler, ok
}

// Store provides persistent definitions without coupling Scheduler to a database.
type Store interface {
	List(context.Context) ([]JobDefinition, error)
}

// Loader synchronizes enabled definitions into a MutableScheduler. Atomic
// Validate/Replace semantics are mandatory; persistent reconciliation never
// degrades to a remove-then-add compatibility path.
type Loader struct {
	store      Store
	registry   HandlerRegistry
	scheduler  MutableScheduler
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
func WithLocker(locker Locker) LoaderOption {
	return func(loader *Loader) { loader.locker = locker }
}

// WithRunRecorder records persistent executions.
func WithRunRecorder(recorder RunRecorder) LoaderOption {
	return func(loader *Loader) { loader.recorder = recorder }
}

// WithInstanceID identifies this application instance in run records.
func WithInstanceID(id string) LoaderOption {
	return func(loader *Loader) { loader.instanceID = strings.TrimSpace(id) }
}

// NewLoader creates a storage-neutral persistent job loader.
func NewLoader(store Store, registry HandlerRegistry, scheduler MutableScheduler, options ...LoaderOption) (*Loader, error) {
	if isNilValue(store) || isNilValue(registry) || isNilValue(scheduler) {
		return nil, fmt.Errorf("store, handler registry, and scheduler are required")
	}
	loader := &Loader{
		store:     store,
		registry:  registry,
		scheduler: scheduler,
		loaded:    make(map[string]loadedJob),
	}
	for _, option := range options {
		if option != nil {
			option(loader)
		}
	}
	if isNilValue(loader.locker) {
		loader.locker = nil
	}
	if isNilValue(loader.recorder) {
		loader.recorder = nil
	}
	return loader, nil
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

func cloneJobDefinition(definition JobDefinition) JobDefinition {
	definition.Payload = append(json.RawMessage(nil), definition.Payload...)
	return definition
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

// Lock represents an acquired distributed lock.
type Lock interface{ Unlock(context.Context) error }

// Locker provides an optional distributed execution lock without naming a backend.
// ttl is the minimum requested lease duration. A zero ttl means the framework has
// no execution deadline; adapters must keep the lock valid until Unlock, renewing
// the backend lease when necessary.
type Locker interface {
	TryLock(context.Context, string, time.Duration) (Lock, bool, error)
}

// RunStatus describes the outcome of one persistent execution.
type RunStatus string

const (
	RunRunning  RunStatus = "running"
	RunSuccess  RunStatus = "success"
	RunFailed   RunStatus = "failed"
	RunSkipped  RunStatus = "skipped"
	RunTimeout  RunStatus = "timeout"
	RunCanceled RunStatus = "canceled"
)

// RunRecord is a storage-neutral execution history record. StartedAt is the
// actual framework execution start time; no synthetic scheduled timestamp is
// reported because the runtime engine does not expose one reliably.
type RunRecord struct {
	RunID      string
	JobID      string
	InstanceID string
	StartedAt  time.Time
	FinishedAt time.Time
	Status     RunStatus
	Error      string
	Duration   time.Duration
}

// RunRecorder records persistent execution start and completion. Recorder
// failures are observability failures: they are returned to scheduler logging
// but never suppress the business handler.
type RunRecorder interface {
	Start(context.Context, RunRecord) error
	Finish(context.Context, RunRecord) error
}
