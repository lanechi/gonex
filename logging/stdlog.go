package logging

import "log"

// NewStdLogger adapts Logger to the standard library log.Logger API.
func NewStdLogger(logger Logger, level Level) *log.Logger {
	return log.New(NewWriter(logger, level), "", 0)
}
