package scheduler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

func (loader *Loader) execute(ctx context.Context, handler PersistentHandler, definition JobDefinition) (resultErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	definition = cloneJobDefinition(definition)
	runID, err := newRunID()
	if err != nil {
		return fmt.Errorf("create persistent run ID: %w", err)
	}
	now := time.Now()
	record := RunRecord{
		RunID:       runID,
		JobID:       definition.ID,
		InstanceID:  loader.instanceID,
		ScheduledAt: now,
		StartedAt:   now,
	}
	if loader.recorder != nil {
		if err := loader.recorder.Start(ctx, record); err != nil {
			return fmt.Errorf("record persistent job %q start: %w", definition.Name, err)
		}
	}

	finish := func(status RunStatus, runErr error) error {
		record.FinishedAt = time.Now()
		record.Duration = record.FinishedAt.Sub(record.StartedAt)
		record.Status = status
		if runErr != nil {
			record.Error = runErr.Error()
		}
		if loader.recorder == nil {
			return runErr
		}
		finishErr := loader.recorder.Finish(context.WithoutCancel(ctx), record)
		if finishErr != nil {
			finishErr = fmt.Errorf("record persistent job %q finish: %w", definition.Name, finishErr)
		}
		return errors.Join(runErr, finishErr)
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			panicErr := fmt.Errorf("persistent job %q panicked: %v", definition.Name, recovered)
			_ = finish(RunFailed, panicErr)
			panic(recovered)
		}
	}()

	var lock Lock
	if definition.ExecutionMode == Singleton {
		acquiredLock, acquired, lockErr := loader.locker.TryLock(
			ctx,
			"scheduler:job:"+definition.ID,
			persistentLockTTL(definition.Timeout),
		)
		if lockErr != nil {
			return finish(RunFailed, fmt.Errorf("lock persistent job %q: %w", definition.Name, lockErr))
		}
		if !acquired {
			return finish(RunSkipped, nil)
		}
		if isNilValue(acquiredLock) {
			return finish(RunFailed, fmt.Errorf("persistent locker acquired job %q without a lock", definition.ID))
		}
		lock = acquiredLock
		defer func() {
			unlockErr := lock.Unlock(context.WithoutCancel(ctx))
			if unlockErr != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("unlock persistent job %q: %w", definition.Name, unlockErr))
			}
		}()
	}

	executionDefinition := cloneJobDefinition(definition)
	payload := append([]byte(nil), definition.Payload...)
	handlerErr := handler(ctx, Execution{
		Definition: executionDefinition,
		Payload:    payload,
	})
	if handlerErr == nil {
		return finish(RunSuccess, nil)
	}
	if errors.Is(handlerErr, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return finish(RunTimeout, handlerErr)
	}
	return finish(RunFailed, handlerErr)
}

func persistentLockTTL(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return 0
	}
	grace := timeout / 10
	if grace < 5*time.Second {
		grace = 5 * time.Second
	}
	return timeout + grace
}

func newRunID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return "run-" + hex.EncodeToString(value[:]), nil
}
