package gormlog

import (
	"time"

	"gorm.io/gorm/logger"
)

// Option configures the GORM logger adapter.
type Option func(*Logger)

// WithLogLevel controls which GORM events are forwarded.
func WithLogLevel(level logger.LogLevel) Option {
	return func(adapter *Logger) { adapter.level = level }
}

// WithSlowThreshold controls when a query is reported as slow. A zero value
// disables slow-query logging.
func WithSlowThreshold(threshold time.Duration) Option {
	return func(adapter *Logger) { adapter.slowThreshold = threshold }
}

// WithIgnoreRecordNotFoundError suppresses ErrRecordNotFound entries.
func WithIgnoreRecordNotFoundError(ignore bool) Option {
	return func(adapter *Logger) { adapter.ignoreRecordNotFoundError = ignore }
}

// WithParameterizedQueries records the preference for parameterized SQL. The
// final SQL string supplied by GORM is logged as-is; GORM controls parameter
// interpolation before Trace is called.
func WithParameterizedQueries(parameterized bool) Option {
	return func(adapter *Logger) { adapter.parameterizedQueries = parameterized }
}
