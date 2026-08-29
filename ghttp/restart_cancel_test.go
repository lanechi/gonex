package ghttp

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRestartRejectsCanceledContextBeforeRuntimeInspection(t *testing.T) {
	server := NewServer()
	manager, ok := server.restartManager.(*serverRestartManager)
	if !ok {
		t.Fatalf("default restart manager = %T", server.restartManager)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := manager.Restart(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Restart canceled error = %v, want context.Canceled", err)
	}
	manager.mu.Lock()
	running := manager.running
	attempt := manager.attempt
	manager.mu.Unlock()
	if running || attempt != nil {
		t.Fatalf("canceled restart published runtime state: running=%v attempt=%v", running, attempt != nil)
	}
}

func TestRestartHandoffCleanupContextIsIndependentAndBounded(t *testing.T) {
	caller, cancelCaller := context.WithCancel(context.Background())
	cancelCaller()
	if !errors.Is(caller.Err(), context.Canceled) {
		t.Fatal("test caller context was not canceled")
	}

	cleanup, cancelCleanup := restartHandoffCleanupContext(time.Second)
	defer cancelCleanup()
	if err := cleanup.Err(); err != nil {
		t.Fatalf("post-handoff cleanup inherited caller cancellation: %v", err)
	}
	deadline, ok := cleanup.Deadline()
	if !ok {
		t.Fatal("post-handoff cleanup context has no deadline")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > time.Second {
		t.Fatalf("post-handoff cleanup deadline remaining=%s", remaining)
	}
}

func TestRestartDetectsRequestContextOwnedByServer(t *testing.T) {
	server := NewServer()
	requestContext := context.WithValue(context.Background(), contextKey{}, &Context{server: server})
	if !restartCalledFromServerRequest(requestContext, server) {
		t.Fatal("server-owned request context was not detected")
	}
	if restartCalledFromServerRequest(context.Background(), server) {
		t.Fatal("background context was treated as a server request")
	}
	other := NewServer()
	if restartCalledFromServerRequest(requestContext, other) {
		t.Fatal("request context from another server was treated as local")
	}
}
