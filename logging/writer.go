package logging

import (
	"context"
	"io"
	"strings"
)

type loggerWriter struct {
	logger Logger
	level  Level
}

// NewWriter adapts a framework Logger to io.Writer APIs such as Gin and
// net/http. Each non-empty line is emitted as one structured log entry.
func NewWriter(logger Logger, level Level) io.Writer {
	if logger == nil {
		logger = NewNopLogger()
	}
	return &loggerWriter{logger: logger, level: level}
}

func (writer *loggerWriter) Write(data []byte) (int, error) {
	text := strings.TrimRight(string(data), "\r\n")
	if text == "" {
		return len(data), nil
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		writer.log(context.Background(), line)
	}
	return len(data), nil
}

func (writer *loggerWriter) log(ctx context.Context, message string) {
	switch writer.level {
	case DebugLevel:
		writer.logger.Debug(ctx, message)
	case WarnLevel:
		writer.logger.Warn(ctx, message)
	case ErrorLevel:
		writer.logger.Error(ctx, message)
	default:
		writer.logger.Info(ctx, message)
	}
}
