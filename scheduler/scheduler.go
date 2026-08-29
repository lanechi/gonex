// Package scheduler provides lifecycle-friendly in-process job scheduling.
//
// Its public contract is independent from the scheduling engine used
// internally, so applications do not depend on third-party scheduler types.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/lanechi/gonex/logging"
)

var (
	// ErrStopped reports an operation attempted after a scheduler has stopped.
	ErrStopped = errors.New("scheduler is stopped")
	// ErrStarted reports a duplicate scheduler start attempt.
	ErrStarted = errors.New("scheduler is already started")
	// ErrDuplicateJob reports a second registration using an existing job name.
	ErrDuplicateJob = errors.New("scheduler job name already exists")
	// ErrJobNotFound reports removal of a job that is not registered.
	ErrJobNotFound = errors.New("scheduler job not found")
)

// manager is the built-in MutableScheduler implementation. It is safe for
// concurrent registration, inspection, execution, replacement, and shutdown.
type manager struct {
	mu               sync.RWMutex
	inner            gocron.Scheduler
	location         *time.Location
	jobs             map[string]*jobRecord
	middleware       []Middleware
	logger           logging.Logger
	loggerConfigured bool
	context          context.Context
	cancel           context.CancelFunc
	started          bool
	stopped          bool
	stopping         bool
	stopAttempt      *stopAttempt
}

type stopAttempt struct {
	done chan struct{}
	err  error
}

type jobRecord struct {
	definition Job
	inner      gocron.Job
	cleanup    []gocron.Job
	gate       *overlapGate
}

type jobSnapshot struct {
	definition Job
	inner      gocron.Job
	gate       *overlapGate
}

// SetDefaultLogger supplies a logger when the scheduler was not constructed
// with WithLogger. ghttp.Server uses it to inherit its own Logger.
func (manager *manager) SetDefaultLogger(logger logging.Logger) {
	if manager == nil || logger == nil {
		return
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if !manager.loggerConfigured {
		manager.logger = logger
	}
}

// Start starts scheduling and derives every job context from ctx as well as
// the engine shutdown context. A duplicate start returns ErrStarted.
func (manager *manager) Start(ctx context.Context) error {
	if manager == nil {
		return ErrStopped
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.stopped || manager.stopping {
		return ErrStopped
	}
	if manager.started {
		return ErrStarted
	}
	inner, err := gocron.NewScheduler(gocron.WithLocation(manager.location))
	if err != nil {
		return fmt.Errorf("create scheduler engine: %w", err)
	}
	manager.inner = inner
	manager.context, manager.cancel = context.WithCancel(ctx)
	for _, record := range manager.jobs {
		if err := manager.installLocked(record); err != nil {
			_ = inner.Shutdown()
			manager.inner = nil
			manager.cancel()
			manager.cancel = nil
			manager.context = context.Background()
			return err
		}
	}
	manager.started = true
	inner.Start()
	return nil
}

// Stop immediately cancels job contexts and prevents future scheduling. It
// does not wait for jobs; use Wait after draining other application work.
func (manager *manager) Stop() {
	if manager == nil {
		return
	}
	manager.mu.Lock()
	if manager.stopped || manager.stopping {
		manager.mu.Unlock()
		return
	}
	attempt := &stopAttempt{done: make(chan struct{})}
	manager.stopping = true
	manager.stopAttempt = attempt
	inner := manager.inner
	cancel := manager.cancel
	if cancel != nil {
		cancel()
	}
	if inner == nil {
		manager.stopped = true
		manager.stopping = false
		close(attempt.done)
		manager.mu.Unlock()
		return
	}
	manager.mu.Unlock()
	go func() {
		err := inner.Shutdown()
		manager.mu.Lock()
		manager.stopped = true
		manager.stopping = false
		attempt.err = err
		close(attempt.done)
		manager.mu.Unlock()
	}()
}

// Wait waits for Stop to finish, or returns ctx.Err when the supplied timeout
// expires. Calling Wait before Stop is a no-op.
func (manager *manager) Wait(ctx context.Context) error {
	if manager == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	manager.mu.RLock()
	attempt := manager.stopAttempt
	manager.mu.RUnlock()
	if attempt == nil {
		return nil
	}
	select {
	case <-attempt.done:
		return attempt.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Add registers a named job. The name must be unique for this manager.
func (manager *manager) Add(job Job) error {
	if manager == nil {
		return ErrStopped
	}
	job = cloneJob(job)
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if err := validateJob(job, manager.location); err != nil {
		return err
	}
	if manager.stopped || manager.stopping {
		return ErrStopped
	}
	if _, exists := manager.jobs[job.Name]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateJob, job.Name)
	}
	record := &jobRecord{definition: job, gate: &overlapGate{policy: job.OverlapPolicy}}
	if manager.started {
		if err := manager.installLocked(record); err != nil {
			return err
		}
	}
	manager.jobs[job.Name] = record
	return nil
}

// Replace atomically updates an existing job while preserving its overlap
// gate. Engine cleanup failures remain attached to the registered record so no
// scheduled engine job becomes invisible to later Remove/Replace attempts.
func (manager *manager) Replace(job Job) error {
	if manager == nil {
		return ErrStopped
	}
	job = cloneJob(job)
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if err := validateJob(job, manager.location); err != nil {
		return err
	}
	if manager.stopped || manager.stopping {
		return ErrStopped
	}
	old, exists := manager.jobs[job.Name]
	if !exists {
		return fmt.Errorf("%w: %s", ErrJobNotFound, job.Name)
	}
	gate := old.gate
	if gate == nil {
		gate = &overlapGate{policy: old.definition.OverlapPolicy}
		old.gate = gate
	}
	replacement := &jobRecord{definition: job, gate: gate}
	if manager.started {
		if err := manager.installLocked(replacement); err != nil {
			return err
		}
		if err := manager.removeEngineJobsLocked(old); err != nil {
			cleanupErr := manager.removeEngineJobsLocked(replacement)
			if replacement.inner != nil {
				old.cleanup = append(old.cleanup, replacement.inner)
				replacement.inner = nil
			}
			if len(replacement.cleanup) > 0 {
				old.cleanup = append(old.cleanup, replacement.cleanup...)
				replacement.cleanup = nil
			}
			return errors.Join(
				fmt.Errorf("replace scheduler job %q: remove old engine job: %w", job.Name, err),
				wrapCleanupError(job.Name, cleanupErr),
			)
		}
	}
	gate.setPolicy(job.OverlapPolicy)
	manager.jobs[job.Name] = replacement
	return nil
}

func wrapCleanupError(name string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("replace scheduler job %q: rollback replacement engine job: %w", name, err)
}

// removeEngineJobsLocked removes every engine handle owned by record and keeps
// failed removals attached to that record for a later cleanup attempt.
func (manager *manager) removeEngineJobsLocked(record *jobRecord) error {
	if record == nil || manager.inner == nil {
		return nil
	}
	var removeErrors []error
	if record.inner != nil {
		if err := manager.inner.RemoveJob(record.inner.ID()); err != nil {
			removeErrors = append(removeErrors, err)
		} else {
			record.inner = nil
		}
	}
	if len(record.cleanup) > 0 {
		remaining := make([]gocron.Job, 0, len(record.cleanup))
		for _, pending := range record.cleanup {
			if pending == nil {
				continue
			}
			if err := manager.inner.RemoveJob(pending.ID()); err != nil {
				removeErrors = append(removeErrors, err)
				remaining = append(remaining, pending)
			}
		}
		record.cleanup = remaining
	}
	return errors.Join(removeErrors...)
}

// Validate checks a job against the exact location and validation rules used
// by this scheduler without changing runtime state.
func (manager *manager) Validate(job Job) error {
	if manager == nil {
		return ErrStopped
	}
	manager.mu.RLock()
	location := manager.location
	stopped := manager.stopped || manager.stopping
	manager.mu.RUnlock()
	if stopped {
		return ErrStopped
	}
	return validateJob(cloneJob(job), location)
}

// Remove deletes a job by name. Removing a job prevents future triggers but
// does not cancel an execution already in progress.
func (manager *manager) Remove(name string) error {
	if manager == nil {
		return ErrJobNotFound
	}
	name = strings.TrimSpace(name)
	manager.mu.Lock()
	defer manager.mu.Unlock()
	record, exists := manager.jobs[name]
	if !exists {
		return fmt.Errorf("%w: %s", ErrJobNotFound, name)
	}
	if manager.started {
		if err := manager.removeEngineJobsLocked(record); err != nil {
			return fmt.Errorf("remove scheduler job %q: %w", name, err)
		}
	}
	delete(manager.jobs, name)
	return nil
}

// Jobs returns an immutable snapshot. All mutable record fields are copied
// while the manager lock is held; engine inspection happens afterward so it
// cannot race with Start installing engine handles into pre-registered jobs.
func (manager *manager) Jobs() []JobInfo {
	if manager == nil {
		return nil
	}
	manager.mu.RLock()
	records := make([]jobSnapshot, 0, len(manager.jobs))
	for _, record := range manager.jobs {
		records = append(records, jobSnapshot{
			definition: cloneJob(record.definition),
			inner:      record.inner,
			gate:       record.gate,
		})
	}
	manager.mu.RUnlock()

	jobs := make([]JobInfo, 0, len(records))
	for _, record := range records {
		var nextRun, lastRun time.Time
		if record.inner != nil {
			nextRun, _ = record.inner.NextRun()
			lastRun, _ = record.inner.LastRunStartedAt()
		}
		running := record.gate != nil && record.gate.isRunning()
		jobs = append(jobs, JobInfo{
			Name:     record.definition.Name,
			Schedule: record.definition.Schedule,
			NextRun:  nextRun,
			LastRun:  lastRun,
			Running:  running,
		})
	}
	sort.Slice(jobs, func(left, right int) bool { return jobs[left].Name < jobs[right].Name })
	return jobs
}

// Use installs middleware around all jobs. Nil middleware is rejected because
// silently changing scheduled work is difficult to diagnose.
func (manager *manager) Use(middleware ...Middleware) error {
	if manager == nil {
		return ErrStopped
	}
	for _, item := range middleware {
		if item == nil {
			return errors.New("scheduler middleware must not be nil")
		}
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.stopped || manager.stopping {
		return ErrStopped
	}
	manager.middleware = append(manager.middleware, middleware...)
	return nil
}

func cloneJob(job Job) Job {
	job.Name = strings.TrimSpace(job.Name)
	job.Middleware = append([]Middleware(nil), job.Middleware...)
	return job
}
