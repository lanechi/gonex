package dao

import (
	"os"
	"path/filepath"
	"testing"

	genfs "github.com/lanechi/gonex/gx/internal/gen/fs"
)

func TestRollbackFailedStagePreservesModuleFilesAfterPublicationCommitted(t *testing.T) {
	root := t.TempDir()
	writeDAOTestFile(t, filepath.Join(root, "go.mod"), "module example.com/old\n")
	moduleBefore, err := snapshotModuleFiles(root)
	if err != nil {
		t.Fatal(err)
	}

	stage := filepath.Join(root, "stage-dao")
	if err := os.MkdirAll(stage, 0o755); err != nil {
		t.Fatal(err)
	}
	writeDAOTestFile(t, filepath.Join(stage, "new.go"), "package dao\n")
	transaction, err := genfs.BeginDirectoryTransaction(root, genfs.DirectorySwap{Stage: stage, Target: "dao"})
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()

	writeDAOTestFile(t, filepath.Join(root, "go.mod"), "module example.com/new\n")
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	if !transaction.Committed() {
		t.Fatal("directory transaction did not report committed publication")
	}

	if err := rollbackFailedStage(root, moduleBefore, transaction); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "module example.com/new\n" {
		t.Fatalf("committed module file was rolled back: %q", content)
	}
	if _, err := os.Stat(filepath.Join(root, "dao", "new.go")); err != nil {
		t.Fatalf("committed DAO output was not preserved: %v", err)
	}
}

func TestRollbackFailedStageRestoresModuleFilesBeforePublicationCommit(t *testing.T) {
	root := t.TempDir()
	writeDAOTestFile(t, filepath.Join(root, "go.mod"), "module example.com/old\n")
	oldDAO := filepath.Join(root, "dao")
	if err := os.MkdirAll(oldDAO, 0o755); err != nil {
		t.Fatal(err)
	}
	writeDAOTestFile(t, filepath.Join(oldDAO, "old.go"), "package dao\n")
	moduleBefore, err := snapshotModuleFiles(root)
	if err != nil {
		t.Fatal(err)
	}

	stage := filepath.Join(root, "stage-dao")
	if err := os.MkdirAll(stage, 0o755); err != nil {
		t.Fatal(err)
	}
	writeDAOTestFile(t, filepath.Join(stage, "new.go"), "package dao\n")
	transaction, err := genfs.BeginDirectoryTransaction(root, genfs.DirectorySwap{Stage: stage, Target: "dao"})
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()

	writeDAOTestFile(t, filepath.Join(root, "go.mod"), "module example.com/new\n")
	if err := rollbackFailedStage(root, moduleBefore, transaction); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "module example.com/old\n" {
		t.Fatalf("uncommitted module file was not restored: %q", content)
	}
	if _, err := os.Stat(filepath.Join(root, "dao", "old.go")); err != nil {
		t.Fatalf("old DAO output was not restored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "dao", "new.go")); !os.IsNotExist(err) {
		t.Fatalf("uncommitted DAO output survived rollback: %v", err)
	}
}

func writeDAOTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
