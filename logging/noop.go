package logging

import "context"

type nopLogger struct{}

// NewNopLogger returns a logger that discards all entries.
func NewNopLogger() Logger { return nopLogger{} }

func (nopLogger) Debug(context.Context, string, ...Field) {}
func (nopLogger) Info(context.Context, string, ...Field)  {}
func (nopLogger) Warn(context.Context, string, ...Field)  {}
func (nopLogger) Error(context.Context, string, ...Field) {}
func (logger nopLogger) With(...Field) Logger             { return logger }
func (logger nopLogger) Named(string) Logger              { return logger }
func (nopLogger) Enabled(Level) bool                      { return false }
func (nopLogger) Sync() error                             { return nil }
