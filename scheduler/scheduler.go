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

// manager is the built-in Scheduler implementation. It is safe for concurrent
// registration, inspection, execution, and shutdown.
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
		return nil
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
	if manager.stopped {
		manager.mu.Unlock()
		return
	}
	if manager.stopping {
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
		manager.stopAttempt = attempt
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

// Add registers a named job. The name must be unique for this Manager.
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

// replace atomically updates an existing built-in job while preserving its
// overlap gate. Persistent Loader uses this private capability so a running old
// version and a newly scheduled version cannot bypass SkipIfRunning/QueueOne.
func (manager *manager) replace(job Job) error {
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
	if old.gate == nil {
		old.gate = &overlapGate{policy: old.definition.OverlapPolicy}
	}
	old.gate.setPolicy(job.OverlapPolicy)
	replacement := &jobRecord{definition: job, gate: old.gate}
	if manager.started {
		if err := manager.installLocked(replacement); err != nil {
			old.gate.setPolicy(old.definition.OverlapPolicy)
			return err
		}
		if old.inner != nil {
			if err := manager.inner.RemoveJob(old.inner.ID()); err != nil {
				_ = manager.inner.RemoveJob(replacement.inner.ID())
				old.gate.setPolicy(old.definition.OverlapPolicy)
				return fmt.Errorf("replace scheduler job %q: remove old job: %w", job.Name, err)
			}
		}
	}
	manager.jobs[job.Name] = replacement
	return nil
}

func (manager *manager) validate(job Job) error {
	if manager == nil {
		return ErrStopped
	}
	return validateJob(cloneJob(job), manager.location)
}

// Remove deletes a job by name. Existing executions receive cancellation when
// Stop is called; removing a job only prevents future scheduling.
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
	if manager.started && record.inner != nil {
		if err := manager.inner.RemoveJob(record.inner.ID()); err != nil {
			return fmt.Errorf("remove scheduler job %q: %w", name, err)
		}
	}
	delete(manager.jobs, name)
	return nil
}

// Jobs returns a snapshot that does not expose mutable engine state.
func (manager *manager) Jobs() []JobInfo {
	if manager == nil {
		return nil
	}
	manager.mu.RLock()
	records := make([]*jobRecord, 0, len(manager.jobs))
	for _, record := range manager.jobs {
		records = append(records, record)
	}
	manager.mu.RUnlock()

	jobs := make([]JobInfo, 0, len(records))
	for _, record := range records {
		var nextRun, lastRun time.Time
		if record.inner != nil {
			nextRun, _ = record.inner.NextRun()
			lastRun, _ = record.inner.LastRunStartedAt()
		}
		running := false
		if record.gate != nil {
			running = record.gate.isRunning()
		}
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
