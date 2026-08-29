package ghttp_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/lanechi/gonex/config"
	"github.com/lanechi/gonex/ghttp"
)

func TestTemplateRendering(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<h1>Hello {{.Name}}</h1>"), 0600); err != nil {
		t.Fatal(err)
	}
	server := ghttp.NewServer()
	if err := server.SetTemplateRoot(root); err != nil {
		t.Fatal(err)
	}
	if err := server.Bind(&pageController{}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/page", nil))
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "text/html; charset=utf-8" || response.Body.String() != "<h1>Hello Lane</h1>" {
		t.Fatalf("template response: status=%d body=%s", response.Code, response.Body.String())
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<p>Reloaded {{.Name}}</p>"), 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		response = httptest.NewRecorder()
		server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/page", nil))
		if response.Code == http.StatusOK && response.Body.String() == "<p>Reloaded Lane</p>" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("reloaded template response: status=%d body=%s", response.Code, response.Body.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestStaticFileAndRootMount(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "asset.js")
	if err := os.WriteFile(filePath, []byte("asset"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("root"), 0600); err != nil {
		t.Fatal(err)
	}
	server := ghttp.NewServer(ghttp.WithLogger(&recordingLogger{}), ghttp.WithOpenAPI(ghttp.OpenAPIOptions{}))
	if err := server.StaticFile("/asset.js", filePath); err != nil {
		t.Fatal(err)
	}
	if err := server.StaticFile("/asset.js", filePath); err == nil {
		t.Fatal("duplicate static file route was accepted")
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/asset.js", nil))
	if response.Code != http.StatusOK || response.Body.String() != "asset" {
		t.Fatalf("static file response: status=%d body=%q", response.Code, response.Body.String())
	}

	rootServer := ghttp.NewServer(ghttp.WithLogger(&recordingLogger{}), ghttp.WithOpenAPI(ghttp.OpenAPIOptions{}))
	if err := rootServer.Static("/", root); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	rootServer.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK || response.Body.String() != "root" {
		t.Fatalf("root static response: status=%d body=%q", response.Code, response.Body.String())
	}
	if err := rootServer.StaticFile("/directory", root); err == nil {
		t.Fatal("directory was accepted as a static file")
	}
}

func TestEmbeddedStaticFSAndSPAFallback(t *testing.T) {
	server := ghttp.NewServer()
	if err := server.StaticFS("/assets", fstest.MapFS{"app.js": &fstest.MapFile{Data: []byte("console.log('ok')")}}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "console.log") {
		t.Fatalf("embedded static response: status=%d body=%s", response.Code, response.Body.String())
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("app"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := server.StaticWithOptions("/app", root, ghttp.StaticOptions{CacheControl: "public, max-age=60", SPAFallback: true}); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/app/client-route", nil))
	if response.Code != http.StatusOK || response.Body.String() != "app" || response.Header().Get("Cache-Control") != "public, max-age=60" {
		t.Fatalf("SPA response: status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestConfiguredStaticExtensionAllowlist(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "asset.txt"), []byte("asset"), 0o600); err != nil {
		t.Fatal(err)
	}

	configuration := config.New()
	configuration.Set("server.static.enabled", true)
	configuration.Set("server.static.root", root)
	configuration.Set("server.static.extensions", []string{"txt"})
	server := ghttp.NewServer(ghttp.WithConfig(configuration), ghttp.WithOpenAPI(ghttp.OpenAPIOptions{}))
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/static/asset.txt", nil))
	if response.Code != http.StatusOK || response.Body.String() != "asset" {
		t.Fatalf("custom configured extension: status=%d body=%q", response.Code, response.Body.String())
	}

	configuration = config.New()
	configuration.Set("server.static.enabled", true)
	configuration.Set("server.static.root", root)
	configuration.Set("server.static.extensions", []string{})
	server = ghttp.NewServer(ghttp.WithConfig(configuration), ghttp.WithOpenAPI(ghttp.OpenAPIOptions{}))
	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/static/asset.txt", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("empty configured extension list: status=%d, want 404", response.Code)
	}
}
