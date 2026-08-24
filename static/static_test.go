package static

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMountDoesNotLeaveGETRouteWhenHEADConflicts(t *testing.T) {
	engine := gin.New()
	engine.HEAD("/assets/:file", func(*gin.Context) {})
	if err := Mount(engine, "/assets", t.TempDir(), Options{}); err == nil {
		t.Fatal("Mount succeeded despite HEAD conflict")
	}
	for _, route := range engine.Routes() {
		if route.Method == http.MethodGet && route.Path == "/assets/*filepath" {
			t.Fatal("failed mount left GET route registered")
		}
	}
}

func TestMountRejectsEncodedOrAmbiguousPrefixes(t *testing.T) {
	for _, prefix := range []string{"assets", "/assets/../files", "/assets%2fprivate", "/assets\\private", "/assets?x=1", "/assets#fragment"} {
		if err := Mount(gin.New(), prefix, t.TempDir(), Options{}); err == nil {
			t.Errorf("Mount accepted prefix %q", prefix)
		}
	}
}

func TestMountRejectsEscapesAndDisallowedExtensions(t *testing.T) {
	root := t.TempDir()
	writeStaticFile(t, root, "index.html", "index")
	writeStaticFile(t, root, "app.JS", "app")
	writeStaticFile(t, root, "secret.txt", "secret")
	outside := filepath.Join(t.TempDir(), "outside.js")
	writeStaticFile(t, filepath.Dir(outside), filepath.Base(outside), "outside")
	if err := os.Symlink(outside, filepath.Join(root, "outside.js")); err != nil {
		t.Fatal(err)
	}
	if !localPathEscapes(root, "outside.js") {
		t.Fatal("local symbolic link was not recognized as an escape")
	}
	engine := gin.New()
	if err := Mount(engine, "/assets", root, Options{SPAFallback: true}); err != nil {
		t.Fatal(err)
	}
	if got := serveStatus(engine, "/assets/app.JS"); got != http.StatusOK {
		t.Errorf("allowed file = %d, want 200", got)
	}
	for _, requestPath := range []string{
		"/assets/secret.txt", "/assets/outside.js", "/assets/../secret.txt",
		"/assets/%2e%2e/secret.txt", "/assets/%252e%252e%252fsecret.txt",
		"/assets/%2Fsecret.js", "/assets/..%5csecret.js",
	} {
		if got := serveStatus(engine, requestPath); got != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", requestPath, got)
		}
	}
}

func TestMountFallbackRequiresAllowedRequestAndIndex(t *testing.T) {
	root := t.TempDir()
	writeStaticFile(t, root, "index.html", "index")
	engine := gin.New()
	if err := Mount(engine, "/app", root, Options{SPAFallback: true}); err != nil {
		t.Fatal(err)
	}
	if got := serveStatus(engine, "/app/dashboard"); got != http.StatusOK {
		t.Fatalf("SPA route = %d, want 200", got)
	}
	if got := serveStatus(engine, "/app/private.txt"); got != http.StatusNotFound {
		t.Fatalf("disallowed extension fell back with status %d", got)
	}

	engine = gin.New()
	if err := Mount(engine, "/app", root, Options{SPAFallback: true, Extensions: []string{"js"}}); err != nil {
		t.Fatal(err)
	}
	if got := serveStatus(engine, "/app/dashboard"); got != http.StatusNotFound {
		t.Fatalf("disallowed index fell back with status %d", got)
	}
}

func TestMountServesIndexAndRejectsNonReadMethods(t *testing.T) {
	root := t.TempDir()
	writeStaticFile(t, root, "index.html", "index")
	engine := gin.New()
	if err := Mount(engine, "/", root, Options{}); err != nil {
		t.Fatal(err)
	}
	if got := serveStatus(engine, "/"); got != http.StatusOK {
		t.Fatalf("root index status = %d, want 200", got)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/index.html", nil)
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("POST static status = %d, want 404", response.Code)
	}
}

func TestMountFileUsesAllowlistAndRejectsSymlink(t *testing.T) {
	directory := t.TempDir()
	filePath := filepath.Join(directory, "page.HTML")
	writeStaticFile(t, directory, "page.HTML", "page")
	engine := gin.New()
	if err := MountFile(engine, "/page", filePath, Options{}); err != nil {
		t.Fatal(err)
	}
	if got := serveStatus(engine, "/page"); got != http.StatusOK {
		t.Fatalf("default allowlist status = %d, want 200", got)
	}

	engine = gin.New()
	if err := MountFile(engine, "/page", filePath, Options{Extensions: []string{}}); err != nil {
		t.Fatal(err)
	}
	if got := serveStatus(engine, "/page"); got != http.StatusNotFound {
		t.Fatalf("empty allowlist status = %d, want 404", got)
	}
	if err := os.Symlink(filePath, filepath.Join(directory, "link.html")); err != nil {
		t.Fatal(err)
	}
	if err := MountFile(gin.New(), "/link", filepath.Join(directory, "link.html"), Options{}); err == nil {
		t.Fatal("MountFile accepted a symbolic link")
	}
}

func TestMountFSRejectsEscapesAndHonorsCustomAllowlist(t *testing.T) {
	root := t.TempDir()
	writeStaticFile(t, root, "index.html", "index")
	writeStaticFile(t, root, "asset.TxT", "asset")
	outside := filepath.Join(t.TempDir(), "outside.txt")
	writeStaticFile(t, filepath.Dir(outside), filepath.Base(outside), "outside")
	if err := os.Symlink(outside, filepath.Join(root, "outside.txt")); err != nil {
		t.Fatal(err)
	}
	var filesystem fs.FS = os.DirFS(root)
	engine := gin.New()
	if err := MountFS(engine, "/files", filesystem, Options{Extensions: []string{".txt"}}); err != nil {
		t.Fatal(err)
	}
	if got := serveStatus(engine, "/files/asset.TxT"); got != http.StatusOK {
		t.Fatalf("custom allowlist status = %d, want 200", got)
	}
	for _, requestPath := range []string{"/files/outside.txt", "/files/%2e%2e/outside.txt", "/files/%5coutside.txt"} {
		if got := serveStatus(engine, requestPath); got != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", requestPath, got)
		}
	}
}

func writeStaticFile(t *testing.T, root, name, content string) {
	t.Helper()
	filePath := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func serveStatus(engine http.Handler, requestPath string) int {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, requestPath, nil)
	engine.ServeHTTP(response, request)
	return response.Code
}
