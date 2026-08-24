package ghttp_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lanechi/gonex/ghttp"
	"github.com/lanechi/gonex/logging"
)

func TestRunAcceptsAddressOverride(t *testing.T) {
	server := ghttp.NewServer(
		ghttp.WithLogger(logging.NewNopLogger()),
		ghttp.WithOpenAPI(false),
	)
	started := make(chan struct{})
	server.OnStarted(func(_ context.Context) error {
		close(started)
		return nil
	})
	done := make(chan error, 1)
	go func() {
		done <- server.Run("127.0.0.1:0")
	}()

	select {
	case err := <-done:
		if strings.Contains(strings.ToLower(err.Error()), "operation not permitted") {
			t.Skipf("sandbox does not permit a loopback listener: %v", err)
		}
		t.Fatalf("server exited before startup: %v", err)
	case <-started:
	}

	if server.Address() != "127.0.0.1:0" || server.HTTPServer().Addr != "127.0.0.1:0" {
		t.Fatalf("address override was not applied: address=%q http=%q", server.Address(), server.HTTPServer().Addr)
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("server shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not stop")
	}
}
