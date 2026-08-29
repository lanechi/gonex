package ghttp_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lanechi/gonex/config"
	"github.com/lanechi/gonex/ghttp"
)

func TestLoadConfigEnvironmentAndServerModules(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("configured-static"), 0600); err != nil {
		t.Fatal(err)
	}
	configurationPath := filepath.Join(t.TempDir(), "config.yaml")
	configurationBody := `
server:
  address: ":file"
  readTimeout: 2s
  writeTimeout: 3s
  idleTimeout: 4s
  maxBodyBytes: 2048
  maxMultipartMemory: 1024
  maxHeaderBytes: 4096
  shutdownTimeout: 5s
  allowedHosts: [api.example.com]
  static:
    enabled: true
    mappings:
      - uri: /configured
        path: ` + root + `
session:
  enabled: true
  name: configured_sid
  ttl: 2h
  path: /
  httpOnly: true
  secure: true
  sameSite: none
cors:
  enabled: true
  allowOrigins: [https://client.example.com]
  allowMethods: [GET]
csrf:
  enabled: true
  secure: true
  sameSite: strict
`
	if err := os.WriteFile(configurationPath, []byte(configurationBody), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SERVER_ADDRESS", ":environment")
	configuration, err := config.Load(configurationPath)
	if err != nil {
		t.Fatal(err)
	}
	if configuration.GetString("server.address") != ":environment" || configuration.GetInt("server.maxHeaderBytes") != 4096 {
		t.Fatalf("loaded configuration: address=%q headers=%d", configuration.GetString("server.address"), configuration.GetInt("server.maxHeaderBytes"))
	}
	server := ghttp.NewServer(ghttp.WithLogger(&recordingLogger{}), ghttp.WithConfig(configuration))
	if err := server.Err(); err != nil {
		t.Fatal(err)
	}
	if server.Config() != configuration || server.Address() != ":environment" || server.HTTPServer().ReadTimeout != 2*time.Second || server.HTTPServer().WriteTimeout != 3*time.Second || server.HTTPServer().IdleTimeout != 4*time.Second || server.HTTPServer().MaxHeaderBytes != 4096 {
		t.Fatalf("server config: address=%q http=%#v", server.Address(), server.HTTPServer())
	}
	if server.SessionManager() == nil {
		t.Fatal("configured session manager is nil")
	}
	cookieOptions := server.SessionManager().CookieOptions()
	if !cookieOptions.Secure || !cookieOptions.HTTPOnly || cookieOptions.SameSite != http.SameSiteNoneMode {
		t.Fatalf("session cookie options=%#v", cookieOptions)
	}

	request := httptest.NewRequest(http.MethodGet, "/configured/", nil)
	request.Host = "api.example.com"
	request.Header.Set("Origin", "https://client.example.com")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "configured-static" || response.Header().Get("Access-Control-Allow-Origin") != "https://client.example.com" {
		t.Fatalf("configured modules: status=%d origin=%q body=%q", response.Code, response.Header().Get("Access-Control-Allow-Origin"), response.Body.String())
	}
}

func TestConfiguredTrustedProxiesRemainActive(t *testing.T) {
	configuration := config.New()
	configuration.Set("server.trustedProxies", []string{"127.0.0.1"})
	logger := &recordingLogger{}
	server := ghttp.NewServer(ghttp.WithConfig(configuration), ghttp.WithLogger(logger))
	if err := server.Bind(&helloController{}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/hello", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("X-Forwarded-For", "198.51.100.7")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("trusted proxy request status=%d body=%s", response.Code, response.Body.String())
	}
	for index, message := range logger.Messages() {
		if message != "request completed" {
			continue
		}
		fields := make(map[string]any)
		for _, field := range logger.FieldEntries(index) {
			fields[field.Key] = field.Value
		}
		if fields["client_ip"] != "198.51.100.7" {
			t.Fatalf("trusted proxy client_ip=%v fields=%#v", fields["client_ip"], fields)
		}
		return
	}
	t.Fatal("request access log was not recorded")
}

func TestDisabledSessionDoesNotSkipFollowingConfiguration(t *testing.T) {
	configuration := config.New()
	configuration.Set("session.enabled", false)
	configuration.Set("server.allowedHosts", []string{"allowed.example.com"})
	server := ghttp.NewServer(ghttp.WithLogger(&recordingLogger{}), ghttp.WithConfig(configuration))
	if server.SessionManager() != nil {
		t.Fatal("disabled session manager is not nil")
	}
	if err := server.Bind(&helloController{}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/hello", nil)
	request.Host = "denied.example.com"
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusMisdirectedRequest {
		t.Fatalf("allowed-host config after disabled session was skipped: status=%d", response.Code)
	}
}

func TestInvalidConfigValuesFailInitialization(t *testing.T) {
	tests := []struct {
		key      string
		value    any
		contains string
	}{
		{"logger.level", "not-a-level", "logger"},
		{"server.shutdownTimeout", "0s", "shutdownTimeout"},
		{"session.storage.type", "cookie", "secret"},
		{"session.storage.type", "unknown", "session.storage.type"},
		{"session.sameSite", "invalid", "SameSite"},
		{"csrf.sameSite", "none", "csrf.secure"},
	}
	for _, test := range tests {
		configuration := config.New()
		configuration.Set(test.key, test.value)
		if test.key == "session.storage.type" && test.value == "cookie" {
			// This case is specifically testing the missing/weak secret error.
			// Satisfy the independent revocation-store requirement so the test
			// reaches the intended validation branch.
			configuration.Set("session.storage.revocation", "memory")
		}
		if strings.HasPrefix(test.key, "csrf.") {
			configuration.Set("csrf.enabled", true)
		}
		server := ghttp.NewServer(ghttp.WithConfig(configuration))
		if server.Err() == nil || !strings.Contains(server.Err().Error(), test.contains) {
			t.Errorf("config %s=%v initialization error=%v, want %q", test.key, test.value, server.Err(), test.contains)
		}
		if err := server.Bind(&helloController{}); err == nil || !strings.Contains(err.Error(), test.contains) {
			t.Errorf("Bind did not expose config %s=%v error: %v", test.key, test.value, err)
		}
	}
}

func TestServerLoadsProjectDefaultConfig(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/server-config-test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "config.yaml"), []byte("server:\n  address: ':default-config'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	server := ghttp.NewServer()
	if err := server.Err(); err != nil {
		t.Fatal(err)
	}
	if server.Address() != ":default-config" || server.Config() == nil {
		t.Fatalf("default server config: address=%q config=%v", server.Address(), server.Config())
	}
}
