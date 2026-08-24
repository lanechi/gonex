package ghttp_test

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lanechi/gonex/ghttp"
)

func TestMultipartFileBinding(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("title", "avatar"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", "avatar.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("content")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	server := ghttp.NewServer()
	if err := server.Bind(&uploadController{}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/upload", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "avatar.txt") {
		t.Fatalf("multipart response: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestRequestBodyLimit(t *testing.T) {
	server := ghttp.NewServer(ghttp.WithRequestLimits(4, 0, 0))
	if err := server.Bind(&bindingController{}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/users/42", strings.NewReader(`{"name":"too-large"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("limit status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestCORS(t *testing.T) {
	server := ghttp.NewServer(ghttp.WithCORS(ghttp.CORSOptions{
		Enabled:      true,
		AllowOrigins: []string{"https://client.example.com"},
	}))
	if err := server.Bind(&helloController{}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/hello", nil)
	request.Header.Set("Origin", "https://client.example.com")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Access-Control-Allow-Origin") != "https://client.example.com" {
		t.Fatalf("CORS response: status=%d origin=%q", response.Code, response.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestTrustedProxyConfiguration(t *testing.T) {
	if err := ghttp.NewServer().SetTrustedProxies(nil); err != nil {
		t.Fatalf("SetTrustedProxies(nil) error=%v", err)
	}
}

type restartManager struct{ called bool }

func (manager *restartManager) Restart(context.Context) error {
	manager.called = true
	return nil
}

func TestRestartManagerBoundary(t *testing.T) {
	if err := ghttp.NewServer().Restart(context.Background()); err != ghttp.ErrServerNotRunning {
		t.Fatalf("default Restart error=%v", err)
	}
	manager := &restartManager{}
	if err := ghttp.NewServer(ghttp.WithRestartManager(manager)).Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !manager.called {
		t.Fatal("restart manager was not called")
	}
}
