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
