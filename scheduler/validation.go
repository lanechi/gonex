package scheduler

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-co-op/gocron/v2"
)

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
