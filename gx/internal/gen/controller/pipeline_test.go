package controller

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanechi/gonex/gx/internal/gen/shared"
)

func TestPipelineRejectsInvalidRenderedOutputBeforeStaging(t *testing.T) {
	root := t.TempDir()
	_, err := formatRendered(Rendered{Discovery: Discovery{Project: Project{Root: root}}, Outputs: []shared.Output{{Path: filepath.Join(root, "invalid.go"), Content: []byte("not go"), Mode: shared.OutputForced}}})
	if err == nil {
		t.Fatal("invalid Go source reached staging")
	}
	if _, statErr := os.Stat(filepath.Join(root, "invalid.go")); !os.IsNotExist(statErr) {
		t.Fatalf("format failure wrote output: %v", statErr)
	}
}
