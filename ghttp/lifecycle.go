package ghttp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/lanechi/gonex/lifecycle"
	"github.com/lanechi/gonex/logging"
	"github.com/lanechi/gonex/scheduler"
)

// LifecycleHook is invoked during server startup or shutdown.
type LifecycleHook func(context.Context) error

type schedulerLoggerSetter interface {
	SetDefaultLogger(logging.Logger)
}

func (server *Server) configureScheduler() {
	if server.scheduler == nil {
		manager, err := scheduler.New(scheduler.WithLocation(server.schedulerLocationOrLocal()))
		if err != nil {
			server.addInitializationError(fmt.Errorf("configure scheduler: %w", err))
			return
		}
		server.scheduler = manager
	}
	if !server.schedulerEnabled {
		return
	}
	if configured, ok := server.scheduler.(schedulerLoggerSetter); ok {
		configured.SetDefaultLogger(server.logger)
	}
	server.lifecycle.OnStart(func(ctx context.Context) error {
		return server.scheduler.Start(ctx)
	})
	server.lifecycle.OnShutdown(func(context.Context) error {
		server.scheduler.Stop()
		return nil
	})
}

func (server *Server) schedulerLocationOrLocal() *time.Location {
	if server.schedulerLocation != nil {
		return server.schedulerLocation
	}
	return time.Local
}

// RunTLS starts the server with a certificate and private key.
func (server *Server) RunTLS(certFile, keyFile string) error {
	return server.runWithSignals(true, certFile, keyFile)
}

// RunContext runs the server until the supplied context is canceled or the
// HTTP server returns an error.
func (server *Server) RunContext(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return server.runContext(ctx, server.tlsEnabled, server.tlsCertFile, server.tlsKeyFile)
}

func (server *Server) runWithSignals(tlsEnabled bool, certFile, keyFile string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return server.runContext(ctx, tlsEnabled, certFile, keyFile)
}

func (server *Server) runContext(ctx context.Context, tlsEnabled bool, certFile, keyFile string) error {
	if err := server.Err(); err != nil {
		return err
	}
	if err := server.beginRun(); err != nil {
		return err
	}
	defer server.endRun()
	if err := server.lifecycle.BeginStart(ctx); err != nil {
		return errors.Join(err, server.cleanupFailedStart())
	}
	listener, err := server.openListener()
	if err != nil {
		return errors.Join(err, server.cleanupFailedStart())
	}
	baseListener := listener
	if tlsEnabled {
		certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			_ = listener.Close()
			return errors.Join(fmt.Errorf("load TLS certificate: %w", err), server.cleanupFailedStart())
		}
		listener = tls.NewListener(listener, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
	}
	server.logListening(tlsEnabled)
	server.listenerMu.Lock()
	server.listener = baseListener
	server.listenerMu.Unlock()
	defer func() {
		server.listenerMu.Lock()
		if server.listener == baseListener {
			server.listener = nil
		}
		server.listenerMu.Unlock()
	}()
	if err := server.lifecycle.MarkStarted(ctx); err != nil {
		_ = listener.Close()
		return errors.Join(err, server.cleanupFailedStart())
	}
	errorsChannel := make(chan error, 1)
	go func() { errorsChannel <- server.httpServer.Serve(listener) }()
	select {
	case err := <-errorsChannel:
		return server.finishServe(err)
	default:
	}
	writeRestartReadySignal()
	select {
	case err := <-errorsChannel:
		return server.finishServe(err)
	case <-ctx.Done():
		server.logger.Named("server").Info(context.Background(), "收到退出信号，开始优雅退出")
		shutdownContext, cancel := context.WithTimeout(context.Background(), server.shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			server.logger.Named("server").Error(context.Background(), "优雅退出失败", logging.Error(err))
			return err
		}
		server.logger.Named("server").Info(context.Background(), "服务已优雅退出")
		return nil
	}
}

func (server *Server) cleanupFailedStart() error {
	shutdownContext, cancel := context.WithTimeout(context.Background(), server.shutdownTimeout)
	defer cancel()
	return server.Shutdown(shutdownContext)
}

func (server *Server) finishServe(serveErr error) error {
	shutdownContext, cancel := context.WithTimeout(context.Background(), server.shutdownTimeout)
	defer cancel()
	shutdownErr := server.Shutdown(shutdownContext)
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	return errors.Join(serveErr, shutdownErr)
}

func (server *Server) openListener() (net.Listener, error) {
	if rawFD := os.Getenv(restartListenerEnv); rawFD != "" {
		fd, err := strconv.Atoi(rawFD)
		if err != nil || fd < 0 {
			return nil, fmt.Errorf("invalid inherited listener fd %q", rawFD)
		}
		file := os.NewFile(uintptr(fd), "gonex-inherited-listener")
		if file == nil {
			return nil, errors.New("inherited listener file is unavailable")
		}
		listener, err := net.FileListener(file)
		_ = file.Close()
		if err != nil {
			return nil, fmt.Errorf("inherit listener: %w", err)
		}
		return listener, nil
	}
	return net.Listen("tcp", server.httpServer.Addr)
}

const (
	restartListenerEnv = "GONEX_RESTART_FD"
	restartReadyEnv    = "GONEX_RESTART_READY_FD"
)

func writeRestartReadySignal() {
	rawFD := os.Getenv(restartReadyEnv)
	if rawFD == "" {
		return
	}
	fd, err := strconv.Atoi(rawFD)
	if err != nil || fd < 0 {
		return
	}
	file := os.NewFile(uintptr(fd), "gonex-restart-ready")
	if file == nil {
		return
	}
	_, _ = file.Write([]byte("ready\n"))
	_ = file.Close()
}

// Close performs a bounded graceful shutdown.
func (server *Server) Close() error {
	shutdownContext, cancel := context.WithTimeout(context.Background(), server.shutdownTimeout)
	defer cancel()
	return server.Shutdown(shutdownContext)
}

// OnStart registers a hook before the listener starts accepting requests.
func (server *Server) OnStart(hook LifecycleHook) { server.lifecycle.OnStart(lifecycle.Hook(hook)) }

// OnStarted registers a hook after startup has completed.
func (server *Server) OnStarted(hook LifecycleHook) { server.lifecycle.OnStarted(lifecycle.Hook(hook)) }

// OnShutdown registers a hook before active requests are drained.
func (server *Server) OnShutdown(hook LifecycleHook) {
	server.lifecycle.OnShutdown(lifecycle.Hook(hook))
}

// OnStop registers a hook after the listener and tracked tasks stop.
func (server *Server) OnStop(hook LifecycleHook) { server.lifecycle.OnStop(lifecycle.Hook(hook)) }

// Go tracks a background task for graceful shutdown.
func (server *Server) Go(task func(context.Context)) { server.lifecycle.Go(task) }
