package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTransactionStagesAndCommitsFiles(t *testing.T) {
	root := t.TempDir()
	transaction, err := NewTransaction(root)
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	if err := transaction.Write("internal/generated/file.go", []byte("package generated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "internal/generated/file.go")); !os.IsNotExist(err) {
		t.Fatalf("staged file was visible before commit: %v", err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(root, "internal/generated/file.go"))
	if err != nil || string(content) != "package generated\n" {
		t.Fatalf("committed content = %q, err = %v", content, err)
	}
}

func TestTransactionRejectsEscapeAndRollback(t *testing.T) {
	root := t.TempDir()
	transaction, err := NewTransaction(root)
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	if err := transaction.Write("../outside.go", []byte("bad"), 0o644); err == nil {
		t.Fatal("path traversal was accepted")
	}
	if err := transaction.Rollback(); err != nil {
		t.Fatal(err)
	}
}

func TestTransactionCommitsWritesAndDeletesTogether(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "internal", "generated", "old.go")
	if err := os.MkdirAll(filepath.Dir(old), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	transaction, err := NewTransaction(root)
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	if err := transaction.Write("internal/generated/new.go", []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Delete("internal/generated/old.go"); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old file survived commit: %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(root, "internal/generated/new.go")); err != nil || string(content) != "new" {
		t.Fatalf("new file = %q, err = %v", content, err)
	}
}

func TestTransactionRejectsSymlinkAndConflictingOperations(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	transaction, err := NewTransaction(root)
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	if err := transaction.Write("link/file.go", []byte("bad"), 0o644); err == nil {
		t.Fatal("symlink escape was accepted")
	}
	if err := transaction.Write("same.go", []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Write("same.go", []byte("two"), 0o644); err == nil {
		t.Fatal("duplicate write was accepted")
	}
	if err := transaction.Delete("same.go"); err == nil {
		t.Fatal("write/delete conflict was accepted")
	}
	if err := transaction.Delete("other.go"); err != nil {
		t.Fatal(err)
	}
	if err := transaction.Delete("other.go"); err == nil {
		t.Fatal("duplicate delete was accepted")
	}
}

func TestTransactionRejectsExistingDirectoryWrite(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	transaction, err := NewTransaction(root)
	if err != nil {
		t.Fatal(err)
	}
	defer transaction.Rollback()
	if err := transaction.Write("folder", []byte("bad"), 0o644); err == nil {
		t.Fatal("write to directory was accepted")
	}
}
