// Package redislog adapts go-redis diagnostic logs to gonex logging.Logger.
// It does not construct, own, or close Redis clients.
package redislog

import (
	"context"
	"fmt"

	"github.com/lanechi/gonex/logging"
)

// Logger forwards go-redis diagnostic messages through a gonex logger.
type Logger struct {
	logger logging.Logger
}

// New creates a go-redis compatible logger. A nil logger disables adapter
// output while still satisfying redis.Logging.
func New(logger logging.Logger) *Logger {
	if logger == nil {
		logger = logging.NewNopLogger()
	}
	return &Logger{logger: logger.Named("redis")}
}

// Printf implements redis.Logging and preserves the operation context.
func (adapter *Logger) Printf(ctx context.Context, format string, values ...interface{}) {
	if adapter == nil || adapter.logger == nil {
		return
	}
	adapter.logger.Debug(ctx, fmt.Sprintf(format, values...))
}
