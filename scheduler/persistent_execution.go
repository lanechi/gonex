package scheduler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

const persistentCleanupTimeout = 5 * time.Second

func (loader *Loader) execute(ctx context.Context, handler PersistentHandler, definition JobDefinition) (resultErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	definition = cloneJobDefinition(definition)
	now := time.Now()
	record := RunRecord{
		JobID:      definition.ID,
		InstanceID: loader.instanceID,
		StartedAt:  now,
		Status:     RunRunning,
	}

	var observationErr error
	if loader.recorder != nil {
		runID, err := newRunID()
		if err != nil {
			observationErr = errors.Join(observationErr, fmt.Errorf("create persistent run ID: %w", err))
		} else {
			record.RunID = runID
		}
		if err := callRecorderStart(ctx, loader.recorder, record); err != nil {
			observationErr = errors.Join(observationErr, fmt.Errorf("record persistent job %q start: %w", definition.Name, err))
		}
	}

	var lock Lock
	finalized := false
	finish := func(status RunStatus, runErr error) error {
		if finalized {
			return errors.Join(runErr, observationErr)
		}
		finalized = true

		if lock != nil {
			unlockErr := callUnlock(lock)
			lock = nil
			if unlockErr != nil {
				runErr = errors.Join(runErr, fmt.Errorf("unlock persistent job %q: %w", definition.Name, unlockErr))
				if status == RunSuccess {
					status = RunFailed
				}
			}
		}

		record.FinishedAt = time.Now()
		record.Duration = record.FinishedAt.Sub(record.StartedAt)
		record.Status = status
		if runErr != nil {
			record.Error = runErr.Error()
		}
		if loader.recorder != nil {
			if finishErr := callRecorderFinish(loader.recorder, record); finishErr != nil {
				observationErr = errors.Join(observationErr, fmt.Errorf("record persistent job %q finish: %w", definition.Name, finishErr))
			}
		}
		return errors.Join(runErr, observationErr)
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			panicErr := fmt.Errorf("persistent job %q panicked: %v", definition.Name, recovered)
			_ = finish(RunFailed, panicErr)
			panic(recovered)
		}
	}()

	if definition.ExecutionMode == Singleton {
		acquiredLock, acquired, lockErr := callTryLock(
			loader.locker,
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
	}

	handlerErr := handler(ctx, Execution{Definition: cloneJobDefinition(definition)})
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(handlerErr, context.DeadlineExceeded) {
		if handlerErr == nil {
			handlerErr = context.DeadlineExceeded
		}
		return finish(RunTimeout, handlerErr)
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(handlerErr, context.Canceled) {
		if handlerErr == nil {
			handlerErr = context.Canceled
		}
		return finish(RunCanceled, handlerErr)
	}
	if handlerErr != nil {
		return finish(RunFailed, handlerErr)
	}
	return finish(RunSuccess, nil)
}

func callRecorderStart(parent context.Context, recorder RunRecorder, record RunRecord) (err error) {
	if recorder == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, persistentCleanupTimeout)
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("recorder start panicked: %v", recovered)
		}
	}()
	return recorder.Start(ctx, record)
}

func callRecorderFinish(recorder RunRecorder, record RunRecord) (err error) {
	if recorder == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), persistentCleanupTimeout)
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("recorder finish panicked: %v", recovered)
		}
	}()
	return recorder.Finish(ctx, record)
}

func callTryLock(locker Locker, ctx context.Context, key string, ttl time.Duration) (lock Lock, acquired bool, err error) {
	if isNilValue(locker) {
		return nil, false, fmt.Errorf("locker is required")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			lock, acquired, err = nil, false, fmt.Errorf("locker panicked: %v", recovered)
		}
	}()
	return locker.TryLock(ctx, key, ttl)
}

func callUnlock(lock Lock) (err error) {
	if isNilValue(lock) {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), persistentCleanupTimeout)
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("unlock panicked: %v", recovered)
		}
	}()
	return lock.Unlock(ctx)
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
