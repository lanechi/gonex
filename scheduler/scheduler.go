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

// Handler performs scheduled work. It must respect ctx cancellation so server
// shutdown and per-job timeouts can complete promptly.
type Handler func(context.Context) error

// Middleware decorates a scheduled job handler.
type Middleware func(Handler) Handler

// Schedule describes when a Job runs.
type Schedule interface {
	isSchedule()
}

// Cron schedules a job from a five- or six-field cron expression. Six-field
// expressions include seconds. A TZ= or CRON_TZ= prefix is supported.
type Cron struct {
	Expr string
}

func (Cron) isSchedule() {}

// Every schedules a job at a fixed duration interval.
type Every struct {
	Duration time.Duration
}

func (Every) isSchedule() {}

// Once schedules a job for one future instant.
type Once struct {
	At time.Time
}

func (Once) isSchedule() {}

// OverlapPolicy controls a trigger received while the same job is executing.
type OverlapPolicy uint8

const (
	// SkipIfRunning drops a trigger while a prior run is still active. It is the
	// default because it bounds concurrent work for each job.
	SkipIfRunning OverlapPolicy = iota
	// AllowOverlap starts every trigger even when a prior run is active.
	AllowOverlap
	// QueueOne remembers at most one missed trigger and runs it after the
	// current invocation completes.
	QueueOne
)

// Job is an application-owned scheduled unit of work.
type Job struct {
	Name           string
	Schedule       Schedule
	Handler        Handler
	Timeout        time.Duration
	OverlapPolicy  OverlapPolicy
	RunImmediately bool
	Middleware     []Middleware
}

// JobInfo is a snapshot of a registered job's observable scheduling state.
type JobInfo struct {
	Name     string
	Schedule Schedule
	NextRun  time.Time
	LastRun  time.Time
	Running  bool
}

// Scheduler is the engine-independent contract managed by ghttp.Server.
// Stop immediately signals cancellation, while Wait observes completion after
// callers finish draining their own work. Add and Remove are safe while the
// scheduler is running; a stopped scheduler cannot be restarted.
type Scheduler interface {
	Start(context.Context) error
	Stop()
	Wait(context.Context) error
	Add(Job) error
	Remove(name string) error
	Jobs() []JobInfo
	Use(...Middleware) error
}

// Option configures a scheduler at construction time.
type Option func(*managerOptions) error

type managerOptions struct {
	location  *time.Location
	logger    logging.Logger
	loggerSet bool
}

// WithLocation sets the default location used for cron expressions without a
// TZ= or CRON_TZ= prefix.
func WithLocation(location *time.Location) Option {
	return func(options *managerOptions) error {
		if location == nil {
			return errors.New("scheduler location must not be nil")
		}
		options.location = location
		return nil
	}
}

// WithLogger sends scheduler lifecycle and job outcome logs through logger.
func WithLogger(logger logging.Logger) Option {
	return func(options *managerOptions) error {
		if logger == nil {
			return errors.New("scheduler logger must not be nil")
		}
		options.logger = logger
		options.loggerSet = true
		return nil
	}
}

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
	gate       overlapGate
}

// New creates an independent scheduler. It does not run jobs until Start is
// called, usually by ghttp.Server during lifecycle startup.
func New(options ...Option) (Scheduler, error) {
	configuration := managerOptions{location: time.Local, logger: logging.NewNopLogger()}
	for _, option := range options {
		if option != nil {
			if err := option(&configuration); err != nil {
				return nil, err
			}
		}
	}
	return &manager{
		jobs:             make(map[string]*jobRecord),
		location:         configuration.location,
		logger:           configuration.logger,
		loggerConfigured: configuration.loggerSet,
		context:          context.Background(),
	}, nil
}

// MustNew creates a scheduler and panics when an Option is invalid. It is
// intended for framework defaults whose options are known constants.
func MustNew(options ...Option) Scheduler {
	manager, err := New(options...)
	if err != nil {
		panic(fmt.Sprintf("create scheduler: %v", err))
	}
	return manager
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
	job.Name = strings.TrimSpace(job.Name)
	job.Middleware = append([]Middleware(nil), job.Middleware...)

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
	record := &jobRecord{definition: job, gate: overlapGate{policy: job.OverlapPolicy}}
	if manager.started {
		if err := manager.installLocked(record); err != nil {
			return err
		}
	}
	manager.jobs[job.Name] = record
	return nil
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
		jobs = append(jobs, JobInfo{
			Name:     record.definition.Name,
			Schedule: record.definition.Schedule,
			NextRun:  nextRun,
			LastRun:  lastRun,
			Running:  record.gate.isRunning(),
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

func (manager *manager) run(record *jobRecord, engineContext context.Context) {
	executed, queued := record.gate.run(func() { manager.execute(record, engineContext) })
	if executed {
		return
	}
	logger := manager.currentLogger()
	if queued {
		logger.Debug(context.Background(), "scheduler job queued", logging.String("job", record.definition.Name))
		return
	}
	logger.Warn(context.Background(), "scheduler job skipped because it is already running", logging.String("job", record.definition.Name))
}

func (manager *manager) execute(record *jobRecord, engineContext context.Context) {
	ctx, cancel := manager.jobContext(engineContext, record.definition.Timeout)
	defer cancel()
	logger := manager.currentLogger()
	started := time.Now()
	logger.Info(ctx, "scheduler job started", logging.String("job", record.definition.Name))
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.Error(ctx, "scheduler job panicked", logging.String("job", record.definition.Name), logging.Duration("duration", time.Since(started)), logging.Any("panic", recovered))
		}
	}()

	handler := record.definition.Handler
	middlewares := manager.middlewareSnapshot(record.definition.Middleware)
	for index := len(middlewares) - 1; index >= 0; index-- {
		handler = middlewares[index](handler)
	}
	if err := handler(ctx); err != nil {
		logger.Error(ctx, "scheduler job failed", logging.String("job", record.definition.Name), logging.Duration("duration", time.Since(started)), logging.Error(err))
		return
	}
	logger.Info(ctx, "scheduler job finished", logging.String("job", record.definition.Name), logging.Duration("duration", time.Since(started)))
}

func (manager *manager) jobContext(engineContext context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if engineContext == nil {
		engineContext = context.Background()
	}
	manager.mu.RLock()
	parent := manager.context
	manager.mu.RUnlock()
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancelParent := context.WithCancel(engineContext)
	stopParent := context.AfterFunc(parent, cancelParent)
	if timeout <= 0 {
		return ctx, func() {
			stopParent()
			cancelParent()
		}
	}
	timed, cancelTimeout := context.WithTimeout(ctx, timeout)
	return timed, func() {
		cancelTimeout()
		stopParent()
		cancelParent()
	}
}

func (manager *manager) installLocked(record *jobRecord) error {
	definition, err := scheduleDefinition(record.definition.Schedule)
	if err != nil {
		return err
	}
	options := []gocron.JobOption{gocron.WithName(record.definition.Name)}
	if record.definition.RunImmediately {
		options = append(options, gocron.WithStartAt(gocron.WithStartImmediately()))
	}
	innerJob, err := manager.inner.NewJob(definition, gocron.NewTask(func(ctx context.Context) {
		manager.run(record, ctx)
	}), options...)
	if err != nil {
		return fmt.Errorf("add scheduler job %q: %w", record.definition.Name, err)
	}
	record.inner = innerJob
	return nil
}

func validateJob(job Job, location *time.Location) error {
	if strings.TrimSpace(job.Name) == "" {
		return errors.New("scheduler job name is required")
	}
	if job.Handler == nil {
		return fmt.Errorf("scheduler job %q handler is required", job.Name)
	}
	if job.Timeout < 0 {
		return fmt.Errorf("scheduler job %q timeout must not be negative", job.Name)
	}
	if job.OverlapPolicy != SkipIfRunning && job.OverlapPolicy != AllowOverlap && job.OverlapPolicy != QueueOne {
		return fmt.Errorf("scheduler job %q has invalid overlap policy", job.Name)
	}
	for _, middleware := range job.Middleware {
		if middleware == nil {
			return fmt.Errorf("scheduler job %q middleware must not be nil", job.Name)
		}
	}
	switch schedule := job.Schedule.(type) {
	case Cron:
		if strings.TrimSpace(schedule.Expr) == "" {
			return fmt.Errorf("scheduler job %q cron expression is required", job.Name)
		}
		if err := gocron.NewDefaultCron(cronHasSeconds(schedule.Expr)).IsValid(schedule.Expr, location, time.Now()); err != nil {
			return fmt.Errorf("scheduler job %q has invalid cron expression: %w", job.Name, err)
		}
	case Every:
		if schedule.Duration <= 0 {
			return fmt.Errorf("scheduler job %q interval must be positive", job.Name)
		}
	case Once:
		if job.RunImmediately {
			return fmt.Errorf("scheduler job %q: Once cannot use RunImmediately", job.Name)
		}
		if schedule.At.IsZero() || !schedule.At.After(time.Now()) {
			return fmt.Errorf("scheduler job %q must run at a future time", job.Name)
		}
	default:
		return fmt.Errorf("scheduler job %q has unsupported schedule %T", job.Name, job.Schedule)
	}
	return nil
}
