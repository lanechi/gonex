package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestZapLoggerLevelsFieldsWithAndNamed(t *testing.T) {
	var output bytes.Buffer
	configuration := DefaultConfig()
	configuration.Level = DebugLevel
	configuration.Format = JSONFormat
	configuration.Caller = false
	configuration.Stacktrace = false
	logger, err := NewWithWriter(configuration, &output)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	child := logger.Named("server").With(String("service", "api"))
	child.Debug(ctx, "debug", Int("count", 1))
	child.Info(ctx, "info", Bool("ready", true))
	child.Warn(ctx, "warn", Duration("elapsed", 2*time.Millisecond))
	child.Error(ctx, "error", Error(errors.New("broken")))

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("entries=%d output=%q", len(lines), output.String())
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["logger"] != "server" || entry["service"] != "api" || entry["count"] != float64(1) || entry["level"] != "debug" {
		t.Fatalf("debug entry=%#v", entry)
	}
	if !child.Enabled(DebugLevel) || !child.Enabled(ErrorLevel) {
		t.Fatal("debug logger did not report enabled levels")
	}

	var inherited bytes.Buffer
	infoConfig := configuration
	infoConfig.Level = InfoLevel
	infoLogger, err := NewWithWriter(infoConfig, &inherited)
	if err != nil {
		t.Fatal(err)
	}
	if infoLogger.Enabled(DebugLevel) || !infoLogger.Enabled(InfoLevel) {
		t.Fatal("level enablement is incorrect")
	}
	infoLogger.With(String("scope", "child")).Info(ctx, "message")
	if strings.Contains(inherited.String(), `"scope":"child"`) == false {
		t.Fatalf("With field was not inherited: %s", inherited.String())
	}
}

func TestConsoleColorAndCaller(t *testing.T) {
	for _, test := range []struct {
		name  string
		color ColorMode
		want  bool
	}{
		{name: "always", color: ColorAlways, want: true},
		{name: "never", color: ColorNever, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			configuration := DefaultConfig()
			configuration.Color = test.color
			configuration.Caller = true
			configuration.Stacktrace = false
			logger, err := NewWithWriter(configuration, &output)
			if err != nil {
				t.Fatal(err)
			}
			logger.Info(context.Background(), "console message")
			if strings.Contains(output.String(), "\x1b") != test.want {
				t.Fatalf("color=%s output=%q", test.color, output.String())
			}
			if !strings.Contains(output.String(), "logger_test.go:") {
				t.Fatalf("caller was not the business call site: %q", output.String())
			}
		})
	}
}

func TestContextWriterAndStdLogger(t *testing.T) {
	logger := NewNopLogger()
	if FromContext(context.Background()) != nil {
		t.Fatal("background context unexpectedly contains a logger")
	}
	ctx := NewContext(context.Background(), logger)
	if FromContext(ctx) != logger {
		t.Fatal("context logger was not restored")
	}

	var recorder recordingLogger
	writer := NewWriter(&recorder, WarnLevel)
	if _, err := writer.Write([]byte("first\nsecond\n")); err != nil {
		t.Fatal(err)
	}
	stdLogger := NewStdLogger(&recorder, ErrorLevel)
	stdLogger.Print("third")
	if got := strings.Join(recorder.messages, ","); got != "first,second,third" {
		t.Fatalf("writer messages=%q", got)
	}
}

type recordingLogger struct {
	messages []string
}

func (logger *recordingLogger) Debug(context.Context, string, ...Field) {}
func (logger *recordingLogger) Info(_ context.Context, message string, _ ...Field) {
	logger.messages = append(logger.messages, message)
}
func (logger *recordingLogger) Warn(_ context.Context, message string, _ ...Field) {
	logger.messages = append(logger.messages, message)
}
func (logger *recordingLogger) Error(_ context.Context, message string, _ ...Field) {
	logger.messages = append(logger.messages, message)
}
func (logger *recordingLogger) With(...Field) Logger { return logger }
func (logger *recordingLogger) Named(string) Logger  { return logger }
func (logger *recordingLogger) Enabled(Level) bool   { return true }
func (logger *recordingLogger) Sync() error          { return nil }
