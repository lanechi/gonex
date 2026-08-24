package template

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManagerWatchesTemplateChanges(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "index.html")
	if err := os.WriteFile(path, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	manager := New()
	t.Cleanup(func() { _ = manager.Close() })
	if err := manager.SetRoot(root); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := manager.Execute(&output, "index.html", nil); err != nil {
		t.Fatal(err)
	}
	if output.String() != "before" {
		t.Fatalf("initial template: %q", output.String())
	}
	if err := os.WriteFile(path, []byte("after"), 0600); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		output.Reset()
		if err := manager.Execute(&output, "index.html", nil); err != nil {
			t.Fatal(err)
		}
		if output.String() == "after" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("template cache was not invalidated: %q", output.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestManagerDoesNotWatchBeforeTemplateUse(t *testing.T) {
	manager := New()
	if manager.watcher != nil {
		t.Fatal("new manager unexpectedly created a watcher")
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestManagerAddFuncRejectsInvalidFunctionsWithoutMutation(t *testing.T) {
	manager := New()
	valid := func(value string) string { return value }
	if err := manager.AddFunc("upper", valid); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		function any
	}{
		{name: "invalid-name", function: valid},
		{name: "invalidSignature", function: func() (string, bool) { return "", false }},
		{name: "typedNil", function: (func(string) string)(nil)},
	} {
		if err := manager.AddFunc(test.name, test.function); err == nil {
			t.Fatalf("AddFunc(%q) succeeded", test.name)
		}
		if _, exists := manager.functions[test.name]; exists {
			t.Fatalf("invalid function %q mutated manager state", test.name)
		}
	}
	if manager.functions["upper"] == nil {
		t.Fatal("valid function was removed")
	}
}
