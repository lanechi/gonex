package static

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMountRejectsParentDirectorySymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeStaticFile(t, outside, "page.html", "outside")
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	if !localPathEscapes(root, "linked/page.html") {
		t.Fatal("parent directory symlink escape was not recognized")
	}

	engine := gin.New()
	if err := Mount(engine, "/assets", root, Options{}); err != nil {
		t.Fatal(err)
	}
	if got := serveStatus(engine, "/assets/linked/page.html"); got != http.StatusNotFound {
		t.Fatalf("parent symlink escape status = %d, want 404", got)
	}
}
