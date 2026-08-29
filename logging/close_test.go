package logging

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCloseClosesPackageOwnedFileOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	configuration := DefaultConfig()
	configuration.Output = path
	configuration.Caller = false
	configuration.Stacktrace = false
	logger, err := New(configuration)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info(context.Background(), "owned-output")
	if err := Close(logger); err != nil {
		t.Fatal(err)
	}
	if err := Close(logger); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "owned-output") {
		t.Fatalf("log file does not contain message: %q", content)
	}
}

func TestDefaultLoggerStandardStreamSyncIsPortable(t *testing.T) {
	configuration := DefaultConfig()
	configuration.Caller = false
	configuration.Stacktrace = false
	logger, err := New(configuration)
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Sync(); err != nil {
		t.Fatalf("default logger sync: %v", err)
	}
	if err := Close(logger); err != nil {
		t.Fatalf("default logger close: %v", err)
	}
}

type closeTrackingWriter struct {
	bytes.Buffer
	closed int
}

func (writer *closeTrackingWriter) Close() error {
	writer.closed++
	return nil
}

func TestCloseDoesNotCloseSuppliedWriter(t *testing.T) {
	writer := &closeTrackingWriter{}
	configuration := DefaultConfig()
	configuration.Caller = false
	configuration.Stacktrace = false
	logger, err := NewWithWriter(configuration, writer)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info(context.Background(), "external-output")
	if err := Close(logger); err != nil {
		t.Fatal(err)
	}
	if writer.closed != 0 {
		t.Fatalf("supplied writer was closed %d times", writer.closed)
	}
}
