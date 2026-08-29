package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTransactionRejectsDirectoryDeleteAndParentChildConflicts(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	transaction, err := NewTransaction(root)
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	if err := transaction.Delete("dir"); err == nil {
		t.Fatal("directory delete was accepted")
	}
	if err := transaction.Write("generated/file.go", []byte("package generated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Delete("generated"); err == nil {
		t.Fatal("parent delete overlapping staged write was accepted")
	}
	if err := transaction.Write("generated/file.go/child.go", []byte("bad"), 0o644); err == nil {
		t.Fatal("child write overlapping staged write was accepted")
	}
}

func TestTransactionCommitRevalidatesSymlinkParents(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "generated")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	transaction, err := NewTransaction(root)
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	if err := transaction.Write("generated/file.go", []byte("package generated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(parent); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, parent); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := transaction.Commit(); err == nil {
		t.Fatal("commit accepted a parent replaced by a symlink")
	}
	if _, err := os.Stat(filepath.Join(outside, "file.go")); !os.IsNotExist(err) {
		t.Fatalf("commit escaped project root: %v", err)
	}
}

func TestDirectoryTransactionRejectsParentChildTargets(t *testing.T) {
	root := t.TempDir()
	stageA := filepath.Join(root, "stage-a")
	stageB := filepath.Join(root, "stage-b")
	if err := os.MkdirAll(stageA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stageB, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := BeginDirectoryTransaction(root,
		DirectorySwap{Stage: stageA, Target: "dao"},
		DirectorySwap{Stage: stageB, Target: "dao/entity"},
	); err == nil {
		t.Fatal("overlapping directory targets were accepted")
	}
}
