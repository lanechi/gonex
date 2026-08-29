// Package lifecycle manages ordered server hooks and background tasks.
package lifecycle

import (
	"context"
	"sync"
)

// Hook is invoked during startup or shutdown.
type Hook func(context.Context) error

type phaseAttempt struct {
	done chan struct{}
	err  error
}

// Lifecycle manages ordered hooks and tracked background tasks.
type Lifecycle struct {
	mu               sync.Mutex
	onStart          []Hook
	onStarted        []Hook
	onShutdown       []Hook
	onStop           []Hook
	startHooksRun    bool
	startRunning     bool
	startAttempt     *phaseAttempt
	started          bool
	starting         bool
	startedAttempt   *phaseAttempt
	shutdown         bool
	shutdownOnce     bool
	shutdownHooksRun bool
	shutdownRunning  bool
	shutdownAttempt  *phaseAttempt
	stopRunning      bool
	stopAttempt      *phaseAttempt
	taskCount        int
	taskDone         chan struct{}
	taskContext      context.Context
	cancelTasks      context.CancelFunc
}

// New creates an independent lifecycle manager.
func New() *Lifecycle {
	contextValue, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	close(done)
	return &Lifecycle{taskContext: contextValue, cancelTasks: cancel, taskDone: done}
}

func (lifecycle *Lifecycle) add(target *[]Hook, hook Hook) {
	if lifecycle == nil || hook == nil {
		return
	}
	lifecycle.mu.Lock()
	*target = append(*target, hook)
	lifecycle.mu.Unlock()
}

func (lifecycle *Lifecycle) OnStart(hook Hook)    { lifecycle.add(&lifecycle.onStart, hook) }
func (lifecycle *Lifecycle) OnStarted(hook Hook)  { lifecycle.add(&lifecycle.onStarted, hook) }
func (lifecycle *Lifecycle) OnShutdown(hook Hook) { lifecycle.add(&lifecycle.onShutdown, hook) }
func (lifecycle *Lifecycle) OnStop(hook Hook)     { lifecycle.add(&lifecycle.onStop, hook) }

// BeginStart runs pre-listener startup hooks once.
func (lifecycle *Lifecycle) BeginStart(ctx context.Context) error {
	if lifecycle == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	lifecycle.mu.Lock()
	if lifecycle.startHooksRun {
		lifecycle.mu.Unlock()
		return nil
	}
	if lifecycle.shutdownOnce || lifecycle.shutdown {
		lifecycle.mu.Unlock()
		return context.Canceled
	}
	if lifecycle.startRunning {
		attempt := lifecycle.startAttempt
		lifecycle.mu.Unlock()
		return waitAttempt(ctx, attempt)
	}
	attempt := &phaseAttempt{done: make(chan struct{})}
	lifecycle.startRunning = true
	lifecycle.startAttempt = attempt
	hooks := append([]Hook(nil), lifecycle.onStart...)
	lifecycle.mu.Unlock()

	var firstError error
	for _, hook := range hooks {
		if err := hook(ctx); err != nil {
			firstError = err
			break
		}
		if err := ctx.Err(); err != nil {
			firstError = err
			break
		}
	}
	if firstError == nil {
		firstError = ctx.Err()
	}

	lifecycle.mu.Lock()
	if firstError == nil && (lifecycle.shutdownOnce || lifecycle.shutdown) {
		firstError = context.Canceled
	}
	lifecycle.startHooksRun = firstError == nil
	lifecycle.startRunning = false
	attempt.err = firstError
	close(attempt.done)
	lifecycle.startAttempt = nil
	lifecycle.mu.Unlock()
	return firstError
}

// MarkStarted runs post-listener startup hooks once.
func (lifecycle *Lifecycle) MarkStarted(ctx context.Context) error {
	if lifecycle == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := lifecycle.BeginStart(ctx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	lifecycle.mu.Lock()
	if lifecycle.started {
		lifecycle.mu.Unlock()
		return nil
	}
	if lifecycle.shutdownOnce || lifecycle.shutdown {
		lifecycle.mu.Unlock()
		return context.Canceled
	}
	if lifecycle.starting {
		attempt := lifecycle.startedAttempt
		lifecycle.mu.Unlock()
		return waitAttempt(ctx, attempt)
	}
	attempt := &phaseAttempt{done: make(chan struct{})}
	lifecycle.starting = true
	lifecycle.startedAttempt = attempt
	hooks := append([]Hook(nil), lifecycle.onStarted...)
	lifecycle.mu.Unlock()

	var firstError error
	for _, hook := range hooks {
		if err := hook(ctx); err != nil {
			firstError = err
			break
		}
		if err := ctx.Err(); err != nil {
			firstError = err
			break
		}
	}
	if firstError == nil {
		firstError = ctx.Err()
	}

	lifecycle.mu.Lock()
	if firstError == nil && (lifecycle.shutdownOnce || lifecycle.shutdown) {
		firstError = context.Canceled
	}
	lifecycle.started = firstError == nil
	lifecycle.starting = false
	attempt.err = firstError
	close(attempt.done)
	lifecycle.startedAttempt = nil
	lifecycle.mu.Unlock()
	return firstError
}

// Start runs pre-start and post-start hooks once. Server uses BeginStart and
// MarkStarted separately so OnStarted observes a bound listener.
func (lifecycle *Lifecycle) Start(ctx context.Context) error {
	if err := lifecycle.BeginStart(ctx); err != nil {
		return err
	}
	return lifecycle.MarkStarted(ctx)
}

// BeginShutdown blocks new startup work, waits for any active startup phase to
// finish, then cancels tracked tasks and runs shutdown hooks once. Startup's
// own result is intentionally not propagated: shutdown intent may cause that
// startup attempt to return context.Canceled, while shutdown itself succeeded.
func (lifecycle *Lifecycle) BeginShutdown(ctx context.Context) error {
	if lifecycle == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	lifecycle.mu.Lock()
	if lifecycle.shutdownRunning {
		attempt := lifecycle.shutdownAttempt
		lifecycle.mu.Unlock()
		return waitAttempt(ctx, attempt)
	}
	if lifecycle.shutdownHooksRun {
		attempt := lifecycle.shutdownAttempt
		lifecycle.mu.Unlock()
		return waitAttempt(ctx, attempt)
	}
	attempt := &phaseAttempt{done: make(chan struct{})}
	lifecycle.shutdownOnce = true
	lifecycle.shutdownRunning = true
	lifecycle.shutdownAttempt = attempt
	startAttempt := lifecycle.startAttempt
	startedAttempt := lifecycle.startedAttempt
	hooks := append([]Hook(nil), lifecycle.onShutdown...)
	if lifecycle.cancelTasks != nil {
		lifecycle.cancelTasks()
	}
	lifecycle.mu.Unlock()

	if err := waitAttemptCompletion(ctx, startAttempt); err != nil {
		lifecycle.finishShutdownAttempt(attempt, err, false)
		return err
	}
	if err := waitAttemptCompletion(ctx, startedAttempt); err != nil {
		lifecycle.finishShutdownAttempt(attempt, err, false)
		return err
	}

	var firstError error
	for _, hook := range hooks {
		if err := hook(ctx); err != nil && firstError == nil {
			firstError = err
		}
	}
	lifecycle.finishShutdownAttempt(attempt, firstError, true)
	return firstError
}

func (lifecycle *Lifecycle) finishShutdownAttempt(attempt *phaseAttempt, err error, hooksRun bool) {
	lifecycle.mu.Lock()
	lifecycle.shutdownRunning = false
	if hooksRun {
		lifecycle.shutdownHooksRun = true
	}
	attempt.err = err
	close(attempt.done)
	lifecycle.mu.Unlock()
}

// Stop runs final lifecycle hooks once, after the shutdown phase has completed.
// Concurrent callers wait for the same stop attempt.
func (lifecycle *Lifecycle) Stop(ctx context.Context) error {
	if lifecycle == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	shutdownErr := lifecycle.BeginShutdown(ctx)
	if ctx.Err() != nil {
		return ctx.Err()
	}

	lifecycle.mu.Lock()
	if !lifecycle.shutdownHooksRun {
		lifecycle.mu.Unlock()
		return shutdownErr
	}
	if lifecycle.stopRunning {
		attempt := lifecycle.stopAttempt
		lifecycle.mu.Unlock()
		return waitAttempt(ctx, attempt)
	}
	if lifecycle.shutdown {
		attempt := lifecycle.stopAttempt
		lifecycle.mu.Unlock()
		if attempt == nil {
			return shutdownErr
		}
		if err := waitAttempt(ctx, attempt); err != nil {
			return err
		}
		return attempt.err
	}
	attempt := &phaseAttempt{done: make(chan struct{})}
	lifecycle.stopRunning = true
	lifecycle.stopAttempt = attempt
	hooks := append([]Hook(nil), lifecycle.onStop...)
	lifecycle.mu.Unlock()

	firstError := shutdownErr
	for _, hook := range hooks {
		if err := hook(ctx); err != nil && firstError == nil {
			firstError = err
		}
	}

	lifecycle.mu.Lock()
	lifecycle.shutdown = true
	lifecycle.stopRunning = false
	attempt.err = firstError
	close(attempt.done)
	lifecycle.mu.Unlock()
	return firstError
}

// Go starts and tracks a background task. Tasks added after shutdown begins are
// rejected. The task receives a context canceled by BeginShutdown.
func (lifecycle *Lifecycle) Go(task func(context.Context)) {
	if lifecycle == nil || task == nil {
		return
	}
	lifecycle.mu.Lock()
	if lifecycle.shutdownOnce || lifecycle.shutdown {
		lifecycle.mu.Unlock()
		return
	}
	if lifecycle.taskCount == 0 {
		lifecycle.taskDone = make(chan struct{})
	}
	lifecycle.taskCount++
	taskContext := lifecycle.taskContext
	lifecycle.mu.Unlock()

	go func() {
		defer lifecycle.taskFinished()
		if taskContext == nil {
			taskContext = context.Background()
		}
		task(taskContext)
	}()
}

func (lifecycle *Lifecycle) taskFinished() {
	lifecycle.mu.Lock()
	defer lifecycle.mu.Unlock()
	if lifecycle.taskCount == 0 {
		return
	}
	lifecycle.taskCount--
	if lifecycle.taskCount == 0 {
		close(lifecycle.taskDone)
	}
}

// Wait waits for all currently tracked tasks to finish or for ctx to expire.
// It allocates no waiter goroutine and has no sync.WaitGroup Add/Wait ordering
// constraint.
func (lifecycle *Lifecycle) Wait(ctx context.Context) error {
	if lifecycle == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	lifecycle.mu.Lock()
	if lifecycle.taskCount == 0 {
		lifecycle.mu.Unlock()
		return nil
	}
	done := lifecycle.taskDone
	lifecycle.mu.Unlock()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func waitAttempt(ctx context.Context, attempt *phaseAttempt) error {
	if attempt == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-attempt.done:
		return attempt.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func waitAttemptCompletion(ctx context.Context, attempt *phaseAttempt) error {
	if attempt == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-attempt.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
