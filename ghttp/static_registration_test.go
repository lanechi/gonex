package ghttp

import (
	"errors"
	"testing"
)

func TestStaticMutationBoundaryRejectsRunningServer(t *testing.T) {
	server := NewServer()
	server.stateMu.Lock()
	server.running = true
	server.stateMu.Unlock()

	called := false
	err := server.mountStatic("/assets", func() error {
		called = true
		return nil
	})
	if !errors.Is(err, ErrServerRunning) {
		t.Fatalf("mountStatic error = %v, want ErrServerRunning", err)
	}
	if called {
		t.Fatal("mountStatic mutated the Gin route tree after the server started")
	}

	called = false
	err = server.mountStaticFile(func() error {
		called = true
		return nil
	})
	if !errors.Is(err, ErrServerRunning) {
		t.Fatalf("mountStaticFile error = %v, want ErrServerRunning", err)
	}
	if called {
		t.Fatal("mountStaticFile mutated the Gin route tree after the server started")
	}
}
