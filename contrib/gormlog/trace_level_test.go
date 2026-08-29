package gormlog

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lanechi/gonex/logging"
	gormlogger "gorm.io/gorm/logger"
)

func TestTraceInfoUsesInfoLevel(t *testing.T) {
	capture := &traceLevelCapture{}
	adapter := New(capture, WithLogLevel(gormlogger.Info))
	adapter.Trace(context.Background(), time.Now(), func() (string, int64) {
		return "SELECT 1", 1
	}, nil)
	if got := capture.info.Load(); got != 1 {
		t.Fatalf("info calls = %d, want 1", got)
	}
	if got := capture.debug.Load(); got != 0 {
		t.Fatalf("debug calls = %d, want 0", got)
	}
}

type traceLevelCapture struct {
	debug atomic.Int32
	info  atomic.Int32
}

func (capture *traceLevelCapture) Debug(context.Context, string, ...logging.Field) {
	capture.debug.Add(1)
}
func (capture *traceLevelCapture) Info(context.Context, string, ...logging.Field) {
	capture.info.Add(1)
}
func (*traceLevelCapture) Warn(context.Context, string, ...logging.Field)  {}
func (*traceLevelCapture) Error(context.Context, string, ...logging.Field) {}
func (capture *traceLevelCapture) With(...logging.Field) logging.Logger     { return capture }
func (capture *traceLevelCapture) Named(string) logging.Logger             { return capture }
func (*traceLevelCapture) Enabled(logging.Level) bool                       { return true }
func (*traceLevelCapture) Sync() error                                      { return nil }
