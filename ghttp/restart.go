package ghttp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

var ErrGracefulRestartUnsupported = errors.New("graceful restart requires a platform restart manager")

var ErrServerNotRunning = errors.New("server is not running")

var ErrServerRunning = errors.New("server is already running")

// RestartManager is the platform boundary for zero-downtime process restart.
// Implementations may coordinate listener FD inheritance and readiness with a
// supervisor without coupling those concerns to the Server core.
type RestartManager interface {
	Restart(ctx context.Context) error
}

type restartAttempt struct {
	done chan struct{}
	err  error
}

type serverRestartManager struct {
	server  *Server
	mu      sync.Mutex
	running bool
	attempt *restartAttempt
}

func (manager *serverRestartManager) Restart(ctx context.Context) error {
	if manager == nil || manager.server == nil {
		return ErrGracefulRestartUnsupported
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	manager.mu.Lock()
	if manager.running {
		attempt := manager.attempt
		manager.mu.Unlock()
		return waitRestartAttempt(ctx, attempt)
	}
	// Cancellation may race with acquiring the serialization lock. Do not
	// publish a new restart attempt, duplicate the listener, or spawn a child
	// process once the caller has canceled the operation.
	if err := ctx.Err(); err != nil {
		manager.mu.Unlock()
		return err
	}
	attempt := &restartAttempt{done: make(chan struct{})}
	manager.running = true
	manager.attempt = attempt
	manager.mu.Unlock()

	err := manager.restart(ctx)
	manager.mu.Lock()
	manager.running = false
	attempt.err = err
	close(attempt.done)
	manager.mu.Unlock()
	return err
}

func waitRestartAttempt(ctx context.Context, attempt *restartAttempt) error {
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

func (manager *serverRestartManager) restart(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	manager.server.listenerMu.RLock()
	listener := manager.server.listener
	manager.server.listenerMu.RUnlock()
	if listener == nil {
		return ErrServerNotRunning
	}
	switch runtime.GOOS {
	case "aix", "darwin", "dragonfly", "freebsd", "linux", "netbsd", "openbsd", "solaris":
	default:
		return ErrGracefulRestartUnsupported
	}
	fileProvider, ok := listener.(interface{ File() (*os.File, error) })
	if !ok {
		return ErrGracefulRestartUnsupported
	}
	listenerFile, err := fileProvider.File()
	if err != nil {
		return fmt.Errorf("duplicate listener: %w", err)
	}
	readyReader, readyWriter, err := os.Pipe()
	if err != nil {
		_ = listenerFile.Close()
		return fmt.Errorf("create restart readiness pipe: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = listenerFile.Close()
		_ = readyReader.Close()
		_ = readyWriter.Close()
		return err
	}
	processEnvironment := restartEnvironment()
	processEnvironment = append(processEnvironment,
		restartListenerEnv+"=3",
		restartReadyEnv+"=4",
	)
	process, err := os.StartProcess(os.Args[0], os.Args[1:], &os.ProcAttr{
		Dir:   "",
		Env:   processEnvironment,
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr, listenerFile, readyWriter},
	})
	_ = listenerFile.Close()
	_ = readyWriter.Close()
	if err != nil {
		_ = readyReader.Close()
		return fmt.Errorf("start replacement process: %w", err)
	}
	defer readyReader.Close()
	ready := make(chan error, 1)
	go func() {
		message, readErr := bufio.NewReader(readyReader).ReadString('\n')
		if readErr != nil {
			ready <- readErr
			return
		}
		if strings.TrimSpace(message) != "ready" {
			ready <- fmt.Errorf("unexpected readiness response %q", strings.TrimSpace(message))
			return
		}
		ready <- nil
	}()
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	select {
	case err := <-ready:
		if err != nil {
			_ = process.Kill()
			_, _ = process.Wait()
			return fmt.Errorf("replacement process did not become ready: %w", err)
		}
	case <-ctx.Done():
		_ = process.Kill()
		_, _ = process.Wait()
		return ctx.Err()
	case <-timer.C:
		_ = process.Kill()
		_, _ = process.Wait()
		return errors.New("replacement process readiness timed out")
	}
	shutdownContext, cancel := context.WithTimeout(ctx, manager.server.shutdownTimeout)
	defer cancel()
	if err := manager.server.Shutdown(shutdownContext); err != nil {
		_ = process.Kill()
		_, _ = process.Wait()
		return err
	}
	go func() { _, _ = process.Wait() }()
	return nil
}

func restartEnvironment() []string {
	environment := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, restartListenerEnv+"=") || strings.HasPrefix(entry, restartReadyEnv+"=") {
			continue
		}
		environment = append(environment, entry)
	}
	return environment
}

// WithRestartManager supplies a platform-specific restart implementation.
func WithRestartManager(manager RestartManager) Option {
	return func(server *Server) {
		if !isNilInterface(manager) {
			server.restartManager = manager
		}
	}
}

// Restart delegates to the configured restart manager.
func (server *Server) Restart(ctx context.Context) error {
	if isNilInterface(server.restartManager) {
		return ErrGracefulRestartUnsupported
	}
	return server.restartManager.Restart(ctx)
}
