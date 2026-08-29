package ghttp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lanechi/gonex/config"
	"github.com/lanechi/gonex/ghttp"
	"github.com/lanechi/gonex/lifecycle"
	"github.com/lanechi/gonex/session"
)

func TestConfigAndRuntimeOptionPrecedence(t *testing.T) {
	configuration := config.New()
	configuration.Set("server.address", ":config")
	configuration.Set("server.readTimeout", "2s")
	configuration.Set("server.maxBodyBytes", 1234)
	configuration.Set("server.openapi.enabled", false)
	configuration.Set("server.swagger.path", "/docs")
	server := ghttp.NewServer(ghttp.WithConfig(configuration), ghttp.WithAddress(":runtime"), ghttp.WithHTTPTimeouts(time.Second, 2*time.Second, 3*time.Second))
	if server.Address() != ":runtime" || server.HTTPServer().ReadTimeout != time.Second || server.HTTPServer().MaxHeaderBytes != 1<<20 {
		t.Fatalf("runtime precedence failed: address=%q read=%s header=%d", server.Address(), server.HTTPServer().ReadTimeout, server.HTTPServer().MaxHeaderBytes)
	}
	if server.HTTPServer().ReadTimeout == 2*time.Second {
		t.Fatal("config overwrote runtime timeout")
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("configured OpenAPI status=%d", response.Code)
	}
	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/docs/", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("configured Swagger response: status=%d", response.Code)
	}
}

func TestConfigAbstraction(t *testing.T) {
	configuration := config.New()
	configuration.SetDefault("server.address", ":8000")
	configuration.Set("server.enabled", true)
	if configuration.GetString("server.address") != ":8000" || !configuration.GetBool("server.enabled") {
		t.Fatal("configuration values were not read correctly")
	}
}

func TestCookieAndMemorySession(t *testing.T) {
	server := ghttp.NewServer(ghttp.WithSessionManager(ghttp.NewSessionManager(nil, "sid", time.Hour)))
	if err := server.Bind(&sessionController{}); err != nil {
		t.Fatal(err)
	}
	first := httptest.NewRecorder()
	server.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/session", nil))
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), `"value":"new"`) {
		t.Fatalf("first session response: status=%d body=%s", first.Code, first.Body.String())
	}
	cookies := first.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != "sid" {
		t.Fatalf("session cookies=%#v", cookies)
	}
	secondRequest := httptest.NewRequest(http.MethodGet, "/session", nil)
	secondRequest.AddCookie(cookies[0])
	second := httptest.NewRecorder()
	server.ServeHTTP(second, secondRequest)
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), `"value":"stored"`) {
		t.Fatalf("second session response: status=%d body=%s", second.Code, second.Body.String())
	}
}

func TestCookieSessionStorageAndSessionLifecycle(t *testing.T) {
	storage := session.NewCookieStorage([]byte("test-secret"))
	server := ghttp.NewServer(ghttp.WithSessionManager(ghttp.NewSessionManager(storage, "sid", time.Hour)))
	if err := server.Bind(&sessionController{}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/session", nil))
	cookies := response.Result().Cookies()
	if response.Code != http.StatusOK || len(cookies) == 0 {
		t.Fatalf("signed session response: status=%d cookies=%#v body=%s", response.Code, cookies, response.Body.String())
	}
	cookie := cookies[len(cookies)-1]
	if cookie.Name != "sid" {
		t.Fatalf("signed session cookie=%#v", cookie)
	}
	values, err := storage.Get(context.Background(), cookie.Value)
	if err != nil || values["value"] != "stored" {
		t.Fatalf("signed session round trip: values=%#v err=%v", values, err)
	}
	tampered := "x" + cookie.Value[1:]
	if _, err := storage.Get(context.Background(), tampered); err != session.ErrNotFound {
		t.Fatalf("tampered cookie error=%v", err)
	}
}

func TestLifecycleHooksAndTrackedTasks(t *testing.T) {
	manager := lifecycle.New()
	events := make([]string, 0, 4)
	add := func(name string) lifecycle.Hook {
		return func(context.Context) error { events = append(events, name); return nil }
	}
	manager.OnStart(add("start"))
	manager.OnStarted(add("started"))
	manager.OnShutdown(add("shutdown"))
	manager.OnStop(add("stop"))
	manager.Go(func(ctx context.Context) { <-ctx.Done() })
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	shutdownContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := manager.BeginShutdown(shutdownContext); err != nil {
		t.Fatal(err)
	}
	if err := manager.Wait(shutdownContext); err != nil {
		t.Fatal(err)
	}
	if err := manager.Stop(shutdownContext); err != nil {
		t.Fatal(err)
	}
	if strings.Join(events, ",") != "start,started,shutdown,stop" {
		t.Fatalf("lifecycle events=%v", events)
	}
}
