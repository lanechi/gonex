package ghttp

import (
	"context"
	"errors"
	"testing"
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
