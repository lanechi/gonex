package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryTransactionRejectsStageTargetOverlapBeforeMutation(t *testing.T) {
	root := t.TempDir()
	stageOne := filepath.Join(root, "stage-one")
	stageTwo := filepath.Join(root, "output", "one", "stage-two")
	if err := os.MkdirAll(stageOne, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stageTwo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stageOne, "one.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stageTwo, "two.txt"), []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}

	transaction, err := BeginDirectoryTransaction(root,
		DirectorySwap{Stage: stageOne, Target: "output/one"},
		DirectorySwap{Stage: stageTwo, Target: "output/two"},
	)
	if err == nil {
		if transaction != nil {
			_ = transaction.Rollback()
		}
		t.Fatal("directory transaction accepted a stage nested under another target")
	}
	if _, statErr := os.Stat(filepath.Join(stageOne, "one.txt")); statErr != nil {
		t.Fatalf("stage one changed before preflight failure: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(stageTwo, "two.txt")); statErr != nil {
		t.Fatalf("stage two changed before preflight failure: %v", statErr)
	}
}

func TestDirectoryTransactionRejectsNestedStagesBeforeMutation(t *testing.T) {
	root := t.TempDir()
	stageParent := filepath.Join(root, "stage-parent")
	stageChild := filepath.Join(stageParent, "child")
	if err := os.MkdirAll(stageChild, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stageChild, "child.txt"), []byte("child"), 0o644); err != nil {
		t.Fatal(err)
	}

	transaction, err := BeginDirectoryTransaction(root,
		DirectorySwap{Stage: stageParent, Target: "output/a"},
		DirectorySwap{Stage: stageChild, Target: "output/b"},
	)
	if err == nil {
		if transaction != nil {
			_ = transaction.Rollback()
		}
		t.Fatal("directory transaction accepted nested stage directories")
	}
	if _, statErr := os.Stat(filepath.Join(stageChild, "child.txt")); statErr != nil {
		t.Fatalf("nested stage changed before preflight failure: %v", statErr)
	}
}
