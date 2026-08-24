package gormlog

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/lanechi/gonex/logging"
	gormlogger "gorm.io/gorm/logger"
)

func TestTraceLevelsAndOptions(t *testing.T) {
	logger := &captureLogger{}
	adapter := New(
		logger,
		WithLogLevel(gormlogger.Info),
		WithSlowThreshold(time.Millisecond),
		WithIgnoreRecordNotFoundError(true),
	)
	ctx := context.Background()
	adapter.Trace(ctx, time.Now(), func() (string, int64) { return "SELECT 1", 1 }, nil)
	if !logger.has("query") || !logger.hasField("query", "sql", "SELECT 1") {
		t.Fatalf("ordinary query was not recorded: %#v", logger.entries)
	}

	logger.clear()
	adapter.Trace(ctx, time.Now().Add(-2*time.Millisecond), func() (string, int64) { return "SELECT slow", 2 }, nil)
	if !logger.has("slow query") {
		t.Fatalf("slow query was not recorded: %#v", logger.entries)
	}

	logger.clear()
	adapter.Trace(ctx, time.Now(), func() (string, int64) { return "SELECT missing", 0 }, gormlogger.ErrRecordNotFound)
	if logger.has("query error") {
		t.Fatalf("record-not-found error was not ignored: %#v", logger.entries)
	}

	logger.clear()
	adapter = adapter.LogMode(gormlogger.Error)
	adapter.Trace(ctx, time.Now(), func() (string, int64) { return "SELECT broken", -1 }, errors.New("broken"))
	if !logger.has("query error") || !logger.hasField("query error", "rows", int64(-1)) {
		t.Fatalf("query error was not recorded: %#v", logger.entries)
	}
}

func TestLogModeReturnsIndependentCopy(t *testing.T) {
	logger := &captureLogger{}
	info := New(logger, WithLogLevel(gormlogger.Info))
	errorLogger := info.LogMode(gormlogger.Error)
	info.Info(context.Background(), "info")
	errorLogger.Info(context.Background(), "suppressed")
	if !logger.has("info") || logger.has("suppressed") {
		t.Fatalf("LogMode mutated shared state: %#v", logger.entries)
	}
}

type captureEntry struct {
	message string
	fields  []logging.Field
}

type captureLogger struct {
	mu      sync.Mutex
	entries []captureEntry
}

func (logger *captureLogger) Debug(_ context.Context, message string, fields ...logging.Field) {
	logger.add(message, fields)
}
func (logger *captureLogger) Info(_ context.Context, message string, fields ...logging.Field) {
	logger.add(message, fields)
}
func (logger *captureLogger) Warn(_ context.Context, message string, fields ...logging.Field) {
	logger.add(message, fields)
}
func (logger *captureLogger) Error(_ context.Context, message string, fields ...logging.Field) {
	logger.add(message, fields)
}
func (logger *captureLogger) With(fields ...logging.Field) logging.Logger {
	return logger
}
func (logger *captureLogger) Named(string) logging.Logger { return logger }
func (logger *captureLogger) Enabled(logging.Level) bool  { return true }
func (logger *captureLogger) Sync() error                 { return nil }
func (logger *captureLogger) add(message string, fields []logging.Field) {
	logger.mu.Lock()
	logger.entries = append(logger.entries, captureEntry{message: message, fields: append([]logging.Field(nil), fields...)})
	logger.mu.Unlock()
}
func (logger *captureLogger) clear() {
	logger.mu.Lock()
	logger.entries = nil
	logger.mu.Unlock()
}
func (logger *captureLogger) has(message string) bool {
	logger.mu.Lock()
	defer logger.mu.Unlock()
	for _, entry := range logger.entries {
		if entry.message == message {
			return true
		}
	}
	return false
}
func (logger *captureLogger) hasField(message, key string, value any) bool {
	logger.mu.Lock()
	defer logger.mu.Unlock()
	for _, entry := range logger.entries {
		if entry.message != message {
			continue
		}
		for _, field := range entry.fields {
			if field.Key == key && fmt.Sprint(field.Value) == fmt.Sprint(value) {
				return true
			}
		}
	}
	return false
}
