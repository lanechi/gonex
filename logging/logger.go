// Package logging defines gonex's logging contract and its built-in Zap
// implementation.
//
// Zap is an implementation detail of this package. Framework consumers only
// need Logger, Field, and Level.
package logging

import "context"

// Logger is the structured logging contract used by the framework.
type Logger interface {
	Debug(ctx context.Context, msg string, fields ...Field)
	Info(ctx context.Context, msg string, fields ...Field)
	Warn(ctx context.Context, msg string, fields ...Field)
	Error(ctx context.Context, msg string, fields ...Field)

	With(fields ...Field) Logger
	Named(name string) Logger

	Enabled(level Level) bool
	Sync() error
}

// Close releases resources owned by a logger implementation when it exposes a
// Close method. Loggers without owned resources remain compatible and are only
// synchronized. Writers supplied through NewWithWriter remain caller-owned.
func Close(logger Logger) error {
	if logger == nil {
		return nil
	}
	if closer, ok := logger.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return logger.Sync()
}
