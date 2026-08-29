package scheduler

import (
	"context"
	"time"

	"github.com/lanechi/gonex/logging"
)

func (manager *manager) run(record *jobRecord, engineContext context.Context) {
	if record == nil || !record.runnable(engineContext) {
		return
	}
	executed, queued := record.gate.runWithToken(record, func() { manager.execute(record, engineContext) })
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
		return ctx, func() { stopParent(); cancelParent() }
	}
	timed, cancelTimeout := context.WithTimeout(ctx, timeout)
	return timed, func() { cancelTimeout(); stopParent(); cancelParent() }
}
