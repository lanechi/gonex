package redislog

import (
	"context"
	"testing"

	"github.com/lanechi/gonex/logging"
	"github.com/redis/go-redis/v9"
)

func TestNewHandlesNilLogger(t *testing.T) {
	logger := New(nil)
	if logger == nil {
		t.Fatal("New(nil) returned nil")
	}
	logger.Printf(context.Background(), "redis %s", "ready")
}

func TestLoggerFormatsMessageAndPreservesContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), contextKey{}, "request")
	recording := &recordingLogger{}
	New(recording).Printf(ctx, "dial %s: %d", "cache", 6379)
	if recording.message != "dial cache: 6379" {
		t.Fatalf("message = %q", recording.message)
	}
	if recording.context != ctx {
		t.Fatal("logger did not receive the Redis context")
	}
}

func TestLoggerImplementsRedisLogging(t *testing.T) {
	redis.SetLogger(New(logging.NewNopLogger()))
}

type contextKey struct{}

type recordingLogger struct {
	context context.Context
	message string
}

func (logger *recordingLogger) Debug(ctx context.Context, message string, _ ...logging.Field) {
	logger.context, logger.message = ctx, message
}
func (*recordingLogger) Info(context.Context, string, ...logging.Field)  {}
func (*recordingLogger) Warn(context.Context, string, ...logging.Field)  {}
func (*recordingLogger) Error(context.Context, string, ...logging.Field) {}
func (logger *recordingLogger) With(...logging.Field) logging.Logger     { return logger }
func (logger *recordingLogger) Named(string) logging.Logger              { return logger }
func (*recordingLogger) Enabled(logging.Level) bool                      { return true }
func (*recordingLogger) Sync() error                                     { return nil }
