package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanechi/gonex/gx/internal/gen/shared"
)

func TestPipelineDoesNotPublishWhenFormattingFails(t *testing.T) {
	root := t.TempDir()
	valid := filepath.Join(root, "internal/service/user.go")
	_, err := (Pipeline{}).Commit(Rendered{
		Discovery: Discovery{Project: Project{Root: root}},
		Outputs: []shared.Output{
			{Path: valid, Content: []byte("package service\n"), Mode: shared.OutputReplacing},
			{Path: filepath.Join(root, "internal/logic/user.go"), Content: []byte("not go"), Mode: shared.OutputForced},
		},
	})
	if err == nil {
		t.Fatal("invalid Go source reached publication")
	}
	if _, statErr := os.Stat(valid); !os.IsNotExist(statErr) {
		t.Fatalf("format failure published earlier output: %v", statErr)
	}
}
