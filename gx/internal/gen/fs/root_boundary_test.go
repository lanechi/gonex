package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTransactionPublicationStaysBoundToOpenedProjectRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	transaction, err := NewTransaction(root)
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	if err := transaction.Write(filepath.Join("generated", "value.go"), []byte("package generated\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	movedRoot := root + "-moved"
	if err := os.Rename(root, movedRoot); err != nil {
		t.Skipf("renaming temporary project root is unavailable: %v", err)
	}
	if err := os.Symlink(outside, root); err != nil {
		_ = os.Rename(movedRoot, root)
		t.Skipf("symlink unavailable: %v", err)
	}
	defer os.Remove(root)
	defer os.RemoveAll(movedRoot)

	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(movedRoot, "generated", "value.go")); err != nil {
		t.Fatalf("generated file was not published through retained project root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "generated", "value.go")); !os.IsNotExist(err) {
		t.Fatalf("generated file escaped through replaced project path: %v", err)
	}
}

func TestDirectoryTransactionRejectsStageOutsideProjectRoot(t *testing.T) {
	root := t.TempDir()
	stage := t.TempDir()
	if _, err := BeginDirectoryTransaction(root, DirectorySwap{Stage: stage, Target: "dao"}); err == nil {
		t.Fatal("directory transaction accepted a stage outside its descriptor root")
	}
}
