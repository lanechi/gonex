package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type preparedDefinition struct {
	definition JobDefinition
	handler    PersistentHandler
	job        Job
}

type schedulerReplacer interface{ replace(Job) error }
type schedulerValidator interface{ validate(Job) error }
type undoOperation func() error

// Sync validates the complete desired state before mutating the runtime
// scheduler, then reconciles it with rollback on any failed operation.
func (loader *Loader) Sync(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	definitions, err := loader.store.List(ctx)
	if err != nil {
		return err
	}
	prepared, err := loader.prepareDefinitions(definitions)
	if err != nil {
		return err
	}

	loader.mu.Lock()
	defer loader.mu.Unlock()

	previous := cloneLoadedJobs(loader.loaded)
	undos := make([]undoOperation, 0)
	rollback := func(cause error) error {
		var rollbackErrors []error
		for index := len(undos) - 1; index >= 0; index-- {
			if undoErr := undos[index](); undoErr != nil {
				rollbackErrors = append(rollbackErrors, undoErr)
			}
		}
		loader.loaded = previous
		return errors.Join(append([]error{cause}, rollbackErrors...)...)
	}

	for id, old := range previous {
		current, exists := prepared[id]
		if !exists || old.Name != current.definition.Name || old.Version == current.definition.Version {
			continue
		}
		oldJob, oldErr := loader.runtimeJob(old.Definition)
		if oldErr != nil {
			return oldErr
		}
		undo, replaceErr := replaceRuntimeJob(loader.scheduler, oldJob, current.job)
		if replaceErr != nil {
			return rollback(replaceErr)
		}
		undos = append(undos, undo)
	}

	for id, old := range previous {
		current, exists := prepared[id]
		if exists && old.Name == current.definition.Name {
			continue
		}
		oldJob, oldErr := loader.runtimeJob(old.Definition)
		if oldErr != nil {
			return rollback(oldErr)
		}
		if err := removePersistentJob(loader.scheduler, old.Name); err != nil {
			return rollback(err)
		}
		undos = append(undos, func() error { return loader.scheduler.Add(oldJob) })
	}

	for id, current := range prepared {
		old, exists := previous[id]
		if exists && old.Name == current.definition.Name {
			continue
		}
		if err := loader.scheduler.Add(current.job); err != nil {
			return rollback(err)
		}
		name := current.definition.Name
		undos = append(undos, func() error { return removePersistentJob(loader.scheduler, name) })
	}

	next := make(map[string]loadedJob, len(prepared))
	for id, current := range prepared {
		definition := cloneJobDefinition(current.definition)
		next[id] = loadedJob{Version: definition.Version, Name: definition.Name, Definition: definition}
	}
	loader.loaded = next
	return nil
}

func (loader *Loader) prepareDefinitions(definitions []JobDefinition) (map[string]preparedDefinition, error) {
	prepared := make(map[string]preparedDefinition, len(definitions))
	ids := make(map[string]struct{}, len(definitions))
	names := make(map[string]string, len(definitions))
	for _, raw := range definitions {
		definition := cloneJobDefinition(raw)
		definition.ID = strings.TrimSpace(definition.ID)
		definition.Name = strings.TrimSpace(definition.Name)
		definition.Handler = strings.TrimSpace(definition.Handler)
		if definition.ID == "" || definition.Name == "" {
			return nil, fmt.Errorf("persistent job ID and name are required")
		}
		if _, exists := ids[definition.ID]; exists {
			return nil, fmt.Errorf("duplicate persistent job ID %q", definition.ID)
		}
		ids[definition.ID] = struct{}{}
		if priorID, exists := names[definition.Name]; exists {
			return nil, fmt.Errorf("duplicate persistent job name %q for IDs %q and %q", definition.Name, priorID, definition.ID)
		}
		names[definition.Name] = definition.ID
		if !definition.Enabled {
			continue
		}
		switch definition.ExecutionMode {
		case Singleton:
			if isNilValue(loader.locker) {
				return nil, fmt.Errorf("persistent job %q requires a Locker for Singleton execution", definition.Name)
			}
		case EveryInstance:
		default:
			return nil, fmt.Errorf("persistent job %q has invalid execution mode %d", definition.Name, definition.ExecutionMode)
		}
		handler, ok := loader.registry.Get(definition.Handler)
		if !ok {
			return nil, fmt.Errorf("persistent handler %q is not registered", definition.Handler)
		}
		job := loader.jobFor(definition, handler)
		if validator, ok := loader.scheduler.(schedulerValidator); ok {
			if err := validator.validate(job); err != nil {
				return nil, fmt.Errorf("persistent job %q: %w", definition.Name, err)
			}
		} else if err := validateJob(job, time.Local); err != nil {
			return nil, fmt.Errorf("persistent job %q: %w", definition.Name, err)
		}
		prepared[definition.ID] = preparedDefinition{definition: definition, handler: handler, job: job}
	}
	return prepared, nil
}

func (loader *Loader) jobFor(definition JobDefinition, handler PersistentHandler) Job {
	definition = cloneJobDefinition(definition)
	return Job{Name: definition.Name, Schedule: definition.Schedule, Timeout: definition.Timeout, OverlapPolicy: definition.OverlapPolicy, Handler: func(jobContext context.Context) error {
		return loader.execute(jobContext, handler, definition)
	}}
}

func (loader *Loader) runtimeJob(definition JobDefinition) (Job, error) {
	handler, ok := loader.registry.Get(definition.Handler)
	if !ok {
		return Job{}, fmt.Errorf("persistent handler %q is not registered", definition.Handler)
	}
	return loader.jobFor(definition, handler), nil
}

func replaceRuntimeJob(runtime Scheduler, oldJob, newJob Job) (undoOperation, error) {
	if replacer, ok := runtime.(schedulerReplacer); ok {
		if err := replacer.replace(newJob); err != nil {
			return nil, err
		}
		return func() error { return replacer.replace(oldJob) }, nil
	}
	if err := removePersistentJob(runtime, oldJob.Name); err != nil {
		return nil, err
	}
	if err := runtime.Add(newJob); err != nil {
		return nil, errors.Join(err, runtime.Add(oldJob))
	}
	return func() error {
		return errors.Join(removePersistentJob(runtime, newJob.Name), runtime.Add(oldJob))
	}, nil
}

func removePersistentJob(runtime Scheduler, name string) error {
	if err := runtime.Remove(name); err != nil && !errors.Is(err, ErrJobNotFound) {
		return err
	}
	return nil
}

func cloneLoadedJobs(source map[string]loadedJob) map[string]loadedJob {
	clone := make(map[string]loadedJob, len(source))
	for id, job := range source {
		job.Definition = cloneJobDefinition(job.Definition)
		clone[id] = job
	}
	return clone
}
