package ghttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/lanechi/gonex/config"
	"github.com/lanechi/gonex/g"
	"github.com/lanechi/gonex/ghttp"
	"github.com/lanechi/gonex/logging"
)

type switchCaptureLogger struct {
	mu       sync.Mutex
	messages []string
}

func (logger *switchCaptureLogger) Debug(_ context.Context, message string, _ ...logging.Field) {
	logger.add(message)
}
func (logger *switchCaptureLogger) Info(_ context.Context, message string, _ ...logging.Field) {
	logger.add(message)
}
func (logger *switchCaptureLogger) Warn(_ context.Context, message string, _ ...logging.Field) {
	logger.add(message)
}
func (logger *switchCaptureLogger) Error(_ context.Context, message string, _ ...logging.Field) {
	logger.add(message)
}
func (logger *switchCaptureLogger) With(...logging.Field) logging.Logger { return logger }
func (logger *switchCaptureLogger) Named(string) logging.Logger          { return logger }

func (logger *switchCaptureLogger) add(message string) {
	logger.mu.Lock()
	logger.messages = append(logger.messages, message)
	logger.mu.Unlock()
}

func (logger *switchCaptureLogger) clear() {
	logger.mu.Lock()
	logger.messages = nil
	logger.mu.Unlock()
}

func (logger *switchCaptureLogger) contains(message string) bool {
	logger.mu.Lock()
	defer logger.mu.Unlock()
	for _, item := range logger.messages {
		if item == message {
			return true
		}
	}
	return false
}

func (logger *switchCaptureLogger) Enabled(logging.Level) bool { return true }
func (logger *switchCaptureLogger) Sync() error                { return nil }

func TestPreInitializationLoggerIsUsedByNewServer(t *testing.T) {
	previousDefault := logging.Default()
	previousInitial := logging.InitialLogger()
	defer func() {
		logging.SetLogger(previousInitial)
		logging.SetDefault(previousDefault)
	}()

	logger := &switchCaptureLogger{}
	if err := g.SetLogger(logger); err != nil {
		t.Fatal(err)
	}
	server := ghttp.NewServer(ghttp.WithOpenAPI(ghttp.OpenAPIOptions{}))
	logger.clear()
	server.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/not-found", nil))
	if !logger.contains("request completed") {
		t.Fatal("pre-initialization logger was not used by the Server")
	}
}

func TestOpenAPIAndSwaggerShareOneSwitch(t *testing.T) {
	server := ghttp.NewServer(ghttp.WithOpenAPI(ghttp.OpenAPIOptions{}))
	assertStatus := func(path string, expected int) {
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != expected {
			t.Fatalf("GET %s status=%d, want %d", path, recorder.Code, expected)
		}
	}

	assertStatus("/openapi.json", http.StatusNotFound)
	assertStatus("/docs/", http.StatusNotFound)

	server.EnableOpenAPI(true)
	assertStatus("/openapi.json", http.StatusOK)
	assertStatus("/docs/", http.StatusOK)

	server.EnableOpenAPI(false)
	assertStatus("/openapi.json", http.StatusNotFound)
	assertStatus("/docs/", http.StatusNotFound)
}

func TestLoggingConfigurationSuppressesAllFrameworkLogs(t *testing.T) {
	configuration := config.New()
	configuration.Set("server.log.enabled", false)
	logger := &switchCaptureLogger{}
	server := ghttp.NewServer(ghttp.WithConfig(configuration), ghttp.WithLogger(logger))
	if err := server.Bind(&helloController{}); err != nil {
		t.Fatal(err)
	}
	server.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/hello", nil))
	if len(logger.messages) != 0 {
		t.Fatalf("logs while disabled by configuration=%v", logger.messages)
	}
}

func TestDocumentationAndLoggingConfigurationLoad(t *testing.T) {
	configuration := config.New()
	configuration.Set("server.openapi.enabled", false)
	configuration.Set("server.log.enabled", false)
	logger := &switchCaptureLogger{}
	server := ghttp.NewServer(ghttp.WithConfig(configuration), ghttp.WithLogger(logger))

	for _, path := range []string{"/openapi.json", "/docs/"} {
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("configured documentation GET %s status=%d", path, recorder.Code)
		}
	}
	logger.clear()
	server.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/not-found", nil))
	if len(logger.messages) != 0 {
		t.Fatalf("configured logging was not disabled: %v", logger.messages)
	}
}
