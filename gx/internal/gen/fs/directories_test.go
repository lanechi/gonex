package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryTransactionRollbackRestoresAllTargets(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"dao", "entity"} {
		directory := filepath.Join(root, name)
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "old.go"), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	stageDAO := filepath.Join(root, "stage-dao")
	stageEntity := filepath.Join(root, "stage-entity")
	for _, directory := range []string{stageDAO, stageEntity} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "new.go"), []byte("new"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	transaction, err := BeginDirectoryTransaction(root,
		DirectorySwap{Stage: stageDAO, Target: "dao"},
		DirectorySwap{Stage: stageEntity, Target: "entity"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := transaction.Rollback(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dao", "entity"} {
		if _, err := os.Stat(filepath.Join(root, name, "old.go")); err != nil {
			t.Fatalf("%s was not restored: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(root, name, "new.go")); !os.IsNotExist(err) {
			t.Fatalf("%s generated output survived rollback", name)
		}
	}
}

func TestDirectoryTransactionRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	stage := filepath.Join(root, "stage")
	if err := os.Mkdir(stage, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := BeginDirectoryTransaction(root, DirectorySwap{Stage: stage, Target: "../outside"}); err == nil {
		t.Fatal("directory traversal was accepted")
	}
}

func TestDirectoryTransactionRejectsDuplicateAndSymlinkTargets(t *testing.T) {
	root := t.TempDir()
	stageA, stageB := filepath.Join(root, "stage-a"), filepath.Join(root, "stage-b")
	if err := os.MkdirAll(stageA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stageB, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := BeginDirectoryTransaction(root,
		DirectorySwap{Stage: stageA, Target: "dao"},
		DirectorySwap{Stage: stageB, Target: "dao"},
	); err == nil {
		t.Fatal("duplicate target was accepted")
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := BeginDirectoryTransaction(root, DirectorySwap{Stage: stageA, Target: "link/dao"}); err == nil {
		t.Fatal("symlink target parent was accepted")
	}
}
