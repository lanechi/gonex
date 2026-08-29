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
	"sync"
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

type acceptReadyListener struct {
	net.Listener
	once  sync.Once
	ready chan struct{}
}

func newAcceptReadyListener(listener net.Listener) *acceptReadyListener {
	return &acceptReadyListener{Listener: listener, ready: make(chan struct{})}
}

func (listener *acceptReadyListener) Accept() (net.Conn, error) {
	listener.once.Do(func() { close(listener.ready) })
	return listener.Listener.Accept()
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
	return server.runWithSignalsAt("", true, certFile, keyFile)
}

// RunContext runs the server until the supplied context is canceled or the
// HTTP server returns an error.
func (server *Server) RunContext(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return server.runContext(ctx, "", server.tlsEnabled, server.tlsCertFile, server.tlsKeyFile)
}

func (server *Server) runWithSignals(tlsEnabled bool, certFile, keyFile string) error {
	return server.runWithSignalsAt("", tlsEnabled, certFile, keyFile)
}

func (server *Server) runWithSignalsAt(address string, tlsEnabled bool, certFile, keyFile string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return server.runContext(ctx, address, tlsEnabled, certFile, keyFile)
}

func (server *Server) runContext(ctx context.Context, address string, tlsEnabled bool, certFile, keyFile string) error {
	if err := server.Err(); err != nil {
		return err
	}
	if err := server.beginRun(address); err != nil {
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
		if err := server.prepareTLSConfig(certFile, keyFile); err != nil {
			_ = baseListener.Close()
			return errors.Join(err, server.cleanupFailedStart())
		}
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

	servingListener := newAcceptReadyListener(baseListener)
	errorsChannel := make(chan error, 1)
	go func() {
		if tlsEnabled {
			// The certificate was loaded before serving, so empty paths make
			// ServeTLS use TLSConfig.Certificates while it still performs the
			// standard library's ALPN/HTTP2 setup.
			errorsChannel <- server.httpServer.ServeTLS(servingListener, "", "")
			return
		}
		errorsChannel <- server.httpServer.Serve(servingListener)
	}()
	select {
	case <-servingListener.ready:
		// Serve/ServeTLS has entered its accept loop. OnStarted hooks may now
		// perform local readiness probes without waiting for runContext.
	case err := <-errorsChannel:
		return server.finishServe(err)
	case <-ctx.Done():
		_ = servingListener.Close()
		<-errorsChannel
		return errors.Join(ctx.Err(), server.cleanupFailedStart())
	}

	if err := server.lifecycle.MarkStarted(ctx); err != nil {
		_ = servingListener.Close()
		<-errorsChannel
		return errors.Join(err, server.cleanupFailedStart())
	}
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

func (server *Server) prepareTLSConfig(certFile, keyFile string) error {
	certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return fmt.Errorf("load TLS certificate: %w", err)
	}
	configuration := server.httpServer.TLSConfig
	if configuration == nil {
		configuration = &tls.Config{}
	} else {
		configuration = configuration.Clone()
	}
	configuration.Certificates = []tls.Certificate{certificate}
	if configuration.MinVersion == 0 || configuration.MinVersion < tls.VersionTLS12 {
		configuration.MinVersion = tls.VersionTLS12
	}
	server.httpServer.TLSConfig = configuration
	return nil
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

// OnStarted registers a hook after the HTTP server has entered its accept loop.
func (server *Server) OnStarted(hook LifecycleHook) { server.lifecycle.OnStarted(lifecycle.Hook(hook)) }

// OnShutdown registers a hook before active requests are drained.
func (server *Server) OnShutdown(hook LifecycleHook) {
	server.lifecycle.OnShutdown(lifecycle.Hook(hook))
}

// OnStop registers a hook after the listener and tracked tasks stop.
func (server *Server) OnStop(hook LifecycleHook) { server.lifecycle.OnStop(lifecycle.Hook(hook)) }

// Go tracks a background task for graceful shutdown.
func (server *Server) Go(task func(context.Context)) { server.lifecycle.Go(task) }
