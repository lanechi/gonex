package template

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSetRootFailurePreservesServingSnapshot(t *testing.T) {
	good := t.TempDir()
	if err := os.WriteFile(filepath.Join(good, "page.html"), []byte("good"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := New()
	defer manager.Close()
	if err := manager.SetRoot(good); err != nil {
		t.Fatal(err)
	}

	bad := t.TempDir()
	if err := os.WriteFile(filepath.Join(bad, "page.html"), []byte("{{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetRoot(bad); err == nil {
		t.Fatal("invalid replacement root was accepted")
	}

	var output bytes.Buffer
	if err := manager.Execute(&output, "page.html", nil); err != nil {
		t.Fatalf("old snapshot stopped serving after failed SetRoot: %v", err)
	}
	if output.String() != "good" {
		t.Fatalf("rendered %q, want old snapshot", output.String())
	}
}
