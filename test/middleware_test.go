package ghttp_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lanechi/gonex/ghttp"
	"github.com/lanechi/gonex/logging"
)

func TestRequestIDRecoveryAndAccessLog(t *testing.T) {
	server := ghttp.NewServer()
	if err := server.Bind(&helloController{}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/hello", nil)
	request.Header.Set("X-Request-ID", "request-123")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Header().Get("X-Request-ID") != "request-123" {
		t.Fatalf("request ID = %q", response.Header().Get("X-Request-ID"))
	}
	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/hello", nil))
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("generated request ID is empty")
	}

	logger := &recordingLogger{}
	server = ghttp.NewServer(ghttp.WithLogger(logger))
	if err := server.Bind(&panicController{}); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "internal server error") || strings.Contains(response.Body.String(), "test panic") {
		t.Fatalf("recovery response: status=%d body=%s", response.Code, response.Body.String())
	}
	panicAccessLogged := false
	for index, message := range logger.Messages() {
		if message != "request completed" {
			continue
		}
		fields := make(map[string]any)
		for _, field := range logger.FieldEntries(index) {
			fields[field.Key] = field.Value
		}
		if fields["path"] == "/panic" && fields["status"] == http.StatusInternalServerError && fields["error"] != "" {
			panicAccessLogged = true
		}
	}
	if !panicAccessLogged {
		t.Fatal("panic request was not recorded as an HTTP 500 access log")
	}

	logger = &recordingLogger{}
	server = ghttp.NewServer(ghttp.WithLogger(logger))
	if err := server.Bind(&helloController{}); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodGet, "/hello", nil)
	request.Header.Set("X-Request-ID", "request-log")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	for index, message := range logger.Messages() {
		if message != "request completed" {
			continue
		}
		fields := make(map[string]any)
		for _, field := range logger.FieldEntries(index) {
			fields[field.Key] = field.Value
		}
		if fields["request_id"] != "request-log" || fields["path"] != "/hello" || fields["status"] != 200 {
			t.Fatalf("access log fields = %#v", fields)
		}
		return
	}
	t.Fatal("access log entry was not recorded")
}

func TestRequestIDCanBeDisabled(t *testing.T) {
	server := ghttp.NewServer(ghttp.WithRequestID(false))
	if err := server.Bind(&helloController{}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/hello", nil))
	if requestID := response.Header().Get("X-Request-ID"); requestID != "" {
		t.Fatalf("request ID = %q, want empty", requestID)
	}
}

func (logger *recordingLogger) Messages() []string                     { return logger.messages }
func (logger *recordingLogger) FieldEntries(index int) []logging.Field { return logger.fields[index] }

func TestHostAndCSRFProtection(t *testing.T) {
	server := ghttp.NewServer(ghttp.WithAllowedHosts("api.example.com"), ghttp.WithCSRF(ghttp.CSRFOptions{Enabled: true}))
	if err := server.Bind(&helloController{}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/hello", nil)
	request.Host = "evil.example.com"
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusMisdirectedRequest {
		t.Fatalf("invalid host status=%d", response.Code)
	}
	request = httptest.NewRequest(http.MethodGet, "/hello", nil)
	request.Host = "api.example.com"
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || len(response.Result().Cookies()) == 0 {
		t.Fatalf("CSRF bootstrap response: status=%d cookies=%v", response.Code, response.Result().Cookies())
	}
	request = httptest.NewRequest(http.MethodPost, "/hello", nil)
	request.Host = "api.example.com"
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF status=%d", response.Code)
	}
}
