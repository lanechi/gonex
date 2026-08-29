package ghttp

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/lanechi/gonex/lifecycle"
)

func TestOnStartedRunsAfterServeAcceptLoop(t *testing.T) {
	server := NewServer(WithAddress("127.0.0.1:0"))
	probeResult := make(chan error, 1)
	server.OnStarted(func(ctx context.Context) error {
		server.listenerMu.RLock()
		listener := server.listener
		server.listenerMu.RUnlock()
		if listener == nil {
			probeResult <- errors.New("listener is unavailable")
			return nil
		}
		client := &http.Client{Timeout: time.Second}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+listener.Addr().String()+"/openapi.json", nil)
		if err != nil {
			probeResult <- err
			return nil
		}
		response, err := client.Do(request)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode != http.StatusOK {
				err = errors.New("readiness probe returned non-200 status")
			}
		}
		probeResult <- err
		return nil
	})

	runDone := make(chan error, 1)
	go func() { runDone <- server.RunContext(context.Background()) }()
	select {
	case err := <-probeResult:
		if err != nil {
			t.Fatalf("OnStarted readiness probe failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnStarted did not complete a readiness probe")
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), time.Second)
	shutdownErr := server.Shutdown(shutdownContext)
	cancel()
	if shutdownErr != nil && !errors.Is(shutdownErr, lifecycle.ErrPhaseInProgress) {
		t.Fatalf("Shutdown error = %v", shutdownErr)
	}
	select {
	case err := <-runDone:
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, lifecycle.ErrPhaseInProgress) {
			t.Fatalf("RunContext error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunContext did not stop")
	}
}

func TestConcurrentRestartReturnsErrRestartInProgress(t *testing.T) {
	manager := &serverRestartManager{server: &Server{}, running: true}
	if err := manager.Restart(context.Background()); !errors.Is(err, ErrRestartInProgress) {
		t.Fatalf("Restart error = %v, want ErrRestartInProgress", err)
	}
}
