package logging

import (
	"fmt"
	"io"
	"strings"
)

// Format selects the Zap encoder.
type Format string

const (
	ConsoleFormat Format = "console"
	JSONFormat    Format = "json"
)

// ColorMode controls level colors in console output.
type ColorMode string

const (
	ColorAuto   ColorMode = "auto"
	ColorAlways ColorMode = "always"
	ColorNever  ColorMode = "never"
)

// Config configures the built-in logger.
type Config struct {
	Level      Level
	Format     Format
	Output     string
	Color      ColorMode
	Caller     bool
	Stacktrace bool
}

// DefaultConfig is the framework's development-friendly default.
func DefaultConfig() Config {
	return Config{
		Level:      InfoLevel,
		Format:     ConsoleFormat,
		Output:     "stdout",
		Color:      ColorAuto,
		Caller:     true,
		Stacktrace: true,
	}
}

// ConfigSource is the small configuration surface needed by the logger.
// It keeps logging independent from the framework's concrete config package.
type ConfigSource interface {
	Get(key string) any
}

// New creates the built-in Zap-backed logger.
func New(configuration Config) (Logger, error) {
	return newZapLogger(configuration, nil)
}

// NewWithWriter creates a Zap-backed logger using writer. It is useful for
// tests and integrations that own their output stream.
func NewWithWriter(configuration Config, writer io.Writer) (Logger, error) {
	if writer == nil {
		return nil, fmt.Errorf("logger writer is nil")
	}
	return newZapLogger(configuration, writer)
}

// NewDefaultLogger creates the built-in logger with DefaultConfig. The
// default configuration is intentionally valid, so a failure falls back to a
// no-op logger rather than making server construction panic.
func NewDefaultLogger() Logger {
	logger, err := New(DefaultConfig())
	if err != nil {
		return NewNopLogger()
	}
	return logger
}

// NewConfiguredLoggerFromConfig returns nil when no logger configuration is
// present. This lets a Server retain its injected logger or default logger.
func NewConfiguredLoggerFromConfig(source ConfigSource) (Logger, error) {
	if source == nil {
		return nil, nil
	}
	keys := []string{
		"logger.level", "logger.format", "logger.output", "logger.color",
		"logger.caller", "logger.stacktrace",
	}
	present := false
	for _, key := range keys {
		if source.Get(key) != nil {
			present = true
			break
		}
	}
	if !present {
		return nil, nil
	}

	configuration := DefaultConfig()
	if value := source.Get("logger.level"); value != nil {
		level, err := ParseLevel(fmt.Sprint(value))
		if err != nil {
			return nil, err
		}
		configuration.Level = level
	}
	if value := source.Get("logger.format"); value != nil {
		configuration.Format = Format(strings.ToLower(strings.TrimSpace(fmt.Sprint(value))))
		if configuration.Format != ConsoleFormat && configuration.Format != JSONFormat {
			return nil, fmt.Errorf("invalid logger format %q", value)
		}
	}
	if value := source.Get("logger.output"); value != nil {
		configuration.Output = strings.TrimSpace(fmt.Sprint(value))
	}
	if value := source.Get("logger.color"); value != nil {
		configuration.Color = ColorMode(strings.ToLower(strings.TrimSpace(fmt.Sprint(value))))
		if configuration.Color != ColorAuto && configuration.Color != ColorAlways && configuration.Color != ColorNever {
			return nil, fmt.Errorf("invalid logger color mode %q", value)
		}
	}
	if value := source.Get("logger.caller"); value != nil {
		configuration.Caller = configBool(value)
	}
	if value := source.Get("logger.stacktrace"); value != nil {
		configuration.Stacktrace = configBool(value)
	}
	return New(configuration)
}

func configBool(value any) bool {
	switch value := value.(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "true") || strings.EqualFold(strings.TrimSpace(value), "yes") || strings.TrimSpace(value) == "1"
	default:
		return false
	}
}
