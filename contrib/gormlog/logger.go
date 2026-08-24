// Package gormlog adapts the optional GORM logger contract to gonex's
// logging.Logger. It is deliberately outside the framework core.
package gormlog

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lanechi/gonex/logging"
	gormlogger "gorm.io/gorm/logger"
)

// Logger implements gorm.io/gorm/logger.Interface without deciding how logs
// are encoded or where they are written.
type Logger struct {
	logger                    logging.Logger
	level                     gormlogger.LogLevel
	slowThreshold             time.Duration
	ignoreRecordNotFoundError bool
	parameterizedQueries      bool
}

// New creates a GORM adapter named "gorm".
func New(logger logging.Logger, options ...Option) gormlogger.Interface {
	if logger == nil {
		logger = logging.Default()
	}
	adapter := &Logger{
		logger:        logger.Named("gorm"),
		level:         gormlogger.Warn,
		slowThreshold: 200 * time.Millisecond,
	}
	for _, option := range options {
		if option != nil {
			option(adapter)
		}
	}
	return adapter
}

// LogMode returns an independent adapter with the requested GORM level.
func (adapter *Logger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	if adapter == nil {
		return New(logging.NewNopLogger(), WithLogLevel(level))
	}
	copy := *adapter
	copy.level = level
	return &copy
}

func (adapter *Logger) Info(ctx context.Context, msg string, data ...interface{}) {
	if adapter == nil || adapter.level < gormlogger.Info {
		return
	}
	adapter.logger.Info(ctx, formatMessage(msg, data...))
}

func (adapter *Logger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if adapter == nil || adapter.level < gormlogger.Warn {
		return
	}
	adapter.logger.Warn(ctx, formatMessage(msg, data...))
}

func (adapter *Logger) Error(ctx context.Context, msg string, data ...interface{}) {
	if adapter == nil || adapter.level < gormlogger.Error {
		return
	}
	adapter.logger.Error(ctx, formatMessage(msg, data...))
}

// Trace forwards GORM's error, slow-query, and ordinary-query events as
// structured fields. The SQL callback is invoked only when an event is
// enabled, matching GORM's lazy formatting behavior.
func (adapter *Logger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if adapter == nil || adapter.level <= gormlogger.Silent || fc == nil {
		return
	}
	elapsed := time.Since(begin)
	sql, rows := "", int64(-1)
	loadQuery := func() {
		if sql == "" && rows == -1 {
			sql, rows = fc()
		}
	}
	baseFields := func() []logging.Field {
		loadQuery()
		return []logging.Field{
			logging.String("sql", sql),
			logging.Int64("rows", rows),
			logging.Duration("duration", elapsed),
		}
	}

	if err != nil && adapter.level >= gormlogger.Error && (!errors.Is(err, gormlogger.ErrRecordNotFound) || !adapter.ignoreRecordNotFoundError) {
		fields := append(baseFields(), logging.Error(err))
		adapter.logger.Error(ctx, "query error", fields...)
		return
	}
	if adapter.slowThreshold > 0 && elapsed > adapter.slowThreshold && adapter.level >= gormlogger.Warn {
		fields := append(baseFields(), logging.Duration("slow_threshold", adapter.slowThreshold))
		adapter.logger.Warn(ctx, "slow query", fields...)
		return
	}
	if adapter.level == gormlogger.Info {
		adapter.logger.Debug(ctx, "query", baseFields()...)
	}
}

// ParamsFilter matches the optional method supported by GORM's built-in
// logger. GORM invokes this when it wants the adapter to decide whether SQL
// parameters should be interpolated.
func (adapter *Logger) ParamsFilter(_ context.Context, sql string, params ...interface{}) (string, []interface{}) {
	if adapter != nil && adapter.parameterizedQueries {
		return sql, nil
	}
	return sql, params
}

func formatMessage(message string, data ...interface{}) string {
	if len(data) == 0 {
		return message
	}
	return fmt.Sprintf(message, data...)
}
