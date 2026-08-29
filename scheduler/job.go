package scheduler

import (
	"context"
	"time"
)

// Handler performs scheduled work. It must respect ctx cancellation so server
// shutdown and per-job timeouts can complete promptly.
type Handler func(context.Context) error

// Middleware decorates a scheduled job handler.
type Middleware func(Handler) Handler

// Schedule describes when a Job runs.
type Schedule interface{ isSchedule() }

// Cron schedules a job from a five- or six-field cron expression.
type Cron struct{ Expr string }

func (Cron) isSchedule() {}

// Every schedules a job at a fixed duration interval.
type Every struct{ Duration time.Duration }

func (Every) isSchedule() {}

// Once schedules a job for one future time.
type Once struct{ At time.Time }

func (Once) isSchedule() {}

// OverlapPolicy controls a trigger received while the same job is executing.
type OverlapPolicy uint8

const (
	SkipIfRunning OverlapPolicy = iota
	AllowOverlap
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

// Scheduler is the engine-independent runtime contract managed by ghttp.Server.
type Scheduler interface {
	Start(context.Context) error
	Stop()
	Wait(context.Context) error
	Add(Job) error
	Remove(name string) error
	Jobs() []JobInfo
	Use(...Middleware) error
}

// MutableScheduler extends Scheduler with the deterministic validation and
// replacement semantics required by persistent reconciliation. Replace must
// preserve per-job overlap state across generations and must leave the old job
// installed if the replacement cannot be committed.
type MutableScheduler interface {
	Scheduler
	Validate(Job) error
	Replace(Job) error
}
