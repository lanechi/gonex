package logging

import "context"

type contextKey struct{}

// NewContext attaches logger to ctx for downstream application code.
func NewContext(ctx context.Context, logger Logger) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextKey{}, logger)
}

// FromContext returns the logger attached by NewContext, or nil when the
// context does not carry one.
func FromContext(ctx context.Context) Logger {
	if ctx == nil {
		return nil
	}
	logger, _ := ctx.Value(contextKey{}).(Logger)
	return logger
}
