package logging

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/mattn/go-isatty"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type zapLogger struct {
	logger *zap.Logger
}

func newZapLogger(configuration Config, supplied io.Writer) (Logger, error) {
	if configuration.Format == "" {
		configuration.Format = ConsoleFormat
	}
	if configuration.Output == "" {
		configuration.Output = "stdout"
	}
	if configuration.Color == "" {
		configuration.Color = ColorAuto
	}
	if configuration.Format != ConsoleFormat && configuration.Format != JSONFormat {
		return nil, errors.New("logger format must be console or json")
	}
	if configuration.Color != ColorAuto && configuration.Color != ColorAlways && configuration.Color != ColorNever {
		return nil, errors.New("logger color must be auto, always, or never")
	}

	writer := supplied
	if writer == nil {
		var err error
		writer, err = openOutput(configuration.Output)
		if err != nil {
			return nil, err
		}
	}

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
	}
	if configuration.Format == ConsoleFormat {
		encoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05.000")
		if useColor(configuration.Color, configuration.Output, writer) {
			encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		} else {
			encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
		}
	}

	var encoder zapcore.Encoder
	if configuration.Format == JSONFormat {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}
	core := zapcore.NewCore(encoder, zapcore.AddSync(writer), zap.NewAtomicLevelAt(toZapLevel(configuration.Level)))
	options := make([]zap.Option, 0, 3)
	if configuration.Caller {
		options = append(options, zap.AddCaller(), zap.AddCallerSkip(1))
	}
	if configuration.Stacktrace {
		options = append(options, zap.AddStacktrace(zapcore.ErrorLevel))
	}
	return &zapLogger{logger: zap.New(core, options...)}, nil
}

func openOutput(output string) (io.Writer, error) {
	switch strings.ToLower(strings.TrimSpace(output)) {
	case "", "stdout":
		return os.Stdout, nil
	case "stderr":
		return os.Stderr, nil
	default:
		return os.OpenFile(output, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	}
}

func useColor(mode ColorMode, output string, writer io.Writer) bool {
	switch mode {
	case ColorAlways:
		return true
	case ColorNever:
		return false
	default:
		if strings.EqualFold(strings.TrimSpace(output), "stderr") {
			file, ok := writer.(*os.File)
			return ok && (isatty.IsTerminal(file.Fd()) || isatty.IsCygwinTerminal(file.Fd()))
		}
		file, ok := writer.(*os.File)
		return ok && (isatty.IsTerminal(file.Fd()) || isatty.IsCygwinTerminal(file.Fd()))
	}
}

func (logger *zapLogger) Debug(_ context.Context, msg string, fields ...Field) {
	logger.logger.Debug(msg, toZapFields(fields)...)
}

func (logger *zapLogger) Info(_ context.Context, msg string, fields ...Field) {
	logger.logger.Info(msg, toZapFields(fields)...)
}

func (logger *zapLogger) Warn(_ context.Context, msg string, fields ...Field) {
	logger.logger.Warn(msg, toZapFields(fields)...)
}

func (logger *zapLogger) Error(_ context.Context, msg string, fields ...Field) {
	logger.logger.Error(msg, toZapFields(fields)...)
}

func (logger *zapLogger) With(fields ...Field) Logger {
	return &zapLogger{logger: logger.logger.With(toZapFields(fields)...)}
}

func (logger *zapLogger) Named(name string) Logger {
	return &zapLogger{logger: logger.logger.Named(name)}
}

func (logger *zapLogger) Enabled(level Level) bool {
	return logger.logger.Core().Enabled(toZapLevel(level))
}

func (logger *zapLogger) Sync() error {
	if err := logger.logger.Sync(); err != nil && !errors.Is(err, syscall.EINVAL) {
		return err
	}
	return nil
}
