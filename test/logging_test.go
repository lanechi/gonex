package ghttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lanechi/gonex/ghttp"
	"github.com/lanechi/gonex/logging"
)

func TestLoggingModuleIsUsableByExternalModules(t *testing.T) {
	var output bytes.Buffer
	configuration := logging.DefaultConfig()
	configuration.Level = logging.DebugLevel
	configuration.Format = logging.JSONFormat
	configuration.Caller = false
	configuration.Stacktrace = false
	logger, err := logging.NewWithWriter(configuration, &output)
	if err != nil {
		t.Fatal(err)
	}
	logger.Named("database").With(logging.String("driver", "test")).Info(context.Background(), "database connected")

	entries := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, output=%q", len(entries), output.String())
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(entries[0]), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["msg"] != "database connected" || entry["logger"] != "database" || entry["driver"] != "test" {
		t.Fatalf("structured database log = %#v", entry)
	}
}

func TestGinOutputUsesTheUnifiedLogger(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.DebugMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })

	logger := &recordingLogger{}
	server := ghttp.NewServer(ghttp.WithLogger(logger), ghttp.WithMode(ghttp.DebugMode))
	if gin.DefaultWriter == nil || gin.DefaultErrorWriter == nil {
		t.Fatal("Gin writers were not configured")
	}
	if err := server.Bind(&helloController{}); err != nil {
		t.Fatal(err)
	}
	if _, err := gin.DefaultWriter.Write([]byte("gin external output\n")); err != nil {
		t.Fatal(err)
	}

	foundRoute := false
	foundWriter := false
	for index, message := range logger.Messages() {
		fields := make(map[string]any)
		for _, field := range logger.FieldEntries(index) {
			fields[field.Key] = field.Value
		}
		if strings.Contains(message, "[GIN-debug] GET") && strings.Contains(message, "/hello") &&
			strings.Contains(message, "handlers)") && fields["path"] == "/hello" && fields["logger"] == "gin" {
			foundRoute = true
		}
		if message == "gin external output" && fields["logger"] == "gin" {
			foundWriter = true
		}
	}
	if !foundRoute || !foundWriter {
		t.Fatalf("Gin logs were not routed to the unified logger: route=%v writer=%v messages=%v", foundRoute, foundWriter, logger.Messages())
	}
}

func TestCustomLoggerFieldAndNameComposition(t *testing.T) {
	logger := &recordingLogger{}
	composed := logger.With(
		logging.String("first", "1"),
	).With(
		logging.String("second", "2"),
	).Named("database").Named("query")
	composed.Info(context.Background(), "composed")
	if len(logger.Messages()) != 1 {
		t.Fatalf("custom logger messages=%v", logger.Messages())
	}
	fields := make(map[string]any)
	for _, field := range logger.FieldEntries(0) {
		fields[field.Key] = field.Value
	}
	if fields["first"] != "1" || fields["second"] != "2" || fields["logger"] != "database.query" {
		t.Fatalf("composed custom logger fields=%#v", fields)
	}
}
