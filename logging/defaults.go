package logging

import "sync"

var globalLoggers struct {
	sync.RWMutex
	initial       Logger
	defaultLogger Logger
}

// SetLogger registers a logger for Servers created after this call. It is
// intended for application startup and does not change already-created
// Servers. Passing nil clears the registration.
func SetLogger(logger Logger) {
	globalLoggers.Lock()
	globalLoggers.initial = logger
	globalLoggers.Unlock()
}

// InitialLogger returns the startup logger, if one was registered.
func InitialLogger() Logger {
	globalLoggers.RLock()
	defer globalLoggers.RUnlock()
	return globalLoggers.initial
}

// SetDefault replaces the process-wide fallback returned by Default.
func SetDefault(logger Logger) {
	globalLoggers.Lock()
	globalLoggers.defaultLogger = logger
	globalLoggers.Unlock()
}

// Default returns the process-wide fallback logger.
func Default() Logger {
	globalLoggers.Lock()
	defer globalLoggers.Unlock()
	if globalLoggers.defaultLogger == nil {
		globalLoggers.defaultLogger = NewDefaultLogger()
	}
	return globalLoggers.defaultLogger
}
