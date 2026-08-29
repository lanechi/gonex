// Package fs provides safe staged file writes for gx generators.
package fs

import (
	"fmt"
	"os"
	"path/filepath"
)

// Transaction stages generated files below Root and publishes them on Commit.
// Rollback removes the staging directory and leaves existing files untouched.
type Transaction struct {
	root    string
	stage   string
	backup  string
	deletes []string
	writes  map[string]struct{}
	open    bool
}

// Delete stages removal of a project-relative file. Existing content remains
// untouched until Commit, where it is backed up with every staged write.
func (transaction *Transaction) Delete(relative string) error {
	if transaction == nil || !transaction.open {
		return fmt.Errorf("file transaction is closed")
	}
	clean := filepath.Clean(relative)
	if _, err := safeProjectPath(transaction.root, clean); err != nil {
		return fmt.Errorf("transaction path %q: %w", relative, err)
	}
	for _, path := range transaction.deletes {
		if pathOverlaps(path, clean) {
			return fmt.Errorf("transaction path %q conflicts with delete %q", relative, path)
		}
	}
	for path := range transaction.writes {
		if pathOverlaps(path, clean) {
			return fmt.Errorf("transaction path %q conflicts with write %q", relative, path)
		}
	}
	if info, err := os.Stat(filepath.Join(transaction.root, clean)); err == nil && info.IsDir() {
		return fmt.Errorf("transaction path %q is an existing directory", relative)
	}
	transaction.deletes = append(transaction.deletes, clean)
	return nil
}

// NewTransaction creates a staging transaction for root.
func NewTransaction(root string) (*Transaction, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve transaction root: %w", err)
	}
	stage, err := os.MkdirTemp(root, ".gx-stage-*")
	if err != nil {
		return nil, fmt.Errorf("create transaction staging: %w", err)
	}
	return &Transaction{root: root, stage: stage, open: true, writes: make(map[string]struct{})}, nil
}

// Write stages content at a project-relative path. Absolute paths and path
// traversal are rejected so generators cannot write outside the project.
func (transaction *Transaction) Write(relative string, content []byte, permission os.FileMode) error {
	if transaction == nil || !transaction.open {
		return fmt.Errorf("file transaction is closed")
	}
	clean := filepath.Clean(relative)
	if _, err := safeProjectPath(transaction.root, clean); err != nil {
		return fmt.Errorf("transaction path %q: %w", relative, err)
	}
	if info, err := os.Stat(filepath.Join(transaction.root, clean)); err == nil && info.IsDir() {
		return fmt.Errorf("transaction path %q is an existing directory", relative)
	}
	for _, path := range transaction.deletes {
		if pathOverlaps(path, clean) {
			return fmt.Errorf("transaction path %q conflicts with delete %q", relative, path)
		}
	}
	for path := range transaction.writes {
		if pathOverlaps(path, clean) {
			return fmt.Errorf("transaction path %q conflicts with write %q", relative, path)
		}
	}
	path := filepath.Join(transaction.stage, clean)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create staging directory for %s: %w", relative, err)
	}
	if permission == 0 {
		permission = 0o644
	}
	if err := os.WriteFile(path, content, permission); err != nil {
		return fmt.Errorf("stage %s: %w", relative, err)
	}
	transaction.writes[clean] = struct{}{}
	return nil
}

// Commit publishes all staged files and closes the transaction. Every target
// path is revalidated immediately before mutation so a parent replaced with a
// symlink after Write/Delete validation cannot redirect the commit.
func (transaction *Transaction) Commit() error {
	if transaction == nil || !transaction.open {
		return fmt.Errorf("file transaction is closed")
	}
	files := make([]string, 0)
	err := filepath.Walk(transaction.stage, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("inspect file transaction: %w", err)
	}
	transaction.backup, err = os.MkdirTemp(transaction.root, ".gx-backup-*")
	if err != nil {
		return fmt.Errorf("create transaction backup: %w", err)
	}
	installed := make([]string, 0, len(files))
	backedUp := make([]string, 0, len(files)+len(transaction.deletes))
	rollback := func() {
		for index := len(installed) - 1; index >= 0; index-- {
			_ = os.Remove(installed[index])
		}
		for index := len(backedUp) - 1; index >= 0; index-- {
			path := backedUp[index]
			relative, _ := filepath.Rel(transaction.backup, path)
			_ = os.Rename(path, filepath.Join(transaction.root, relative))
		}
	}
	for _, path := range files {
		relative, err := filepath.Rel(transaction.stage, path)
		if err != nil {
			rollback()
			return fmt.Errorf("resolve staged file: %w", err)
		}
		destination, err := safeProjectPath(transaction.root, relative)
		if err != nil {
			rollback()
			return fmt.Errorf("revalidate destination %s: %w", relative, err)
		}
		if _, statErr := os.Stat(destination); statErr == nil {
			backup := filepath.Join(transaction.backup, relative)
			if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
				rollback()
				return fmt.Errorf("create transaction backup directory: %w", err)
			}
			if err := os.Rename(destination, backup); err != nil {
				rollback()
				return fmt.Errorf("backup %s: %w", relative, err)
			}
			backedUp = append(backedUp, backup)
		} else if !os.IsNotExist(statErr) {
			rollback()
			return fmt.Errorf("inspect destination %s: %w", relative, statErr)
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			rollback()
			return fmt.Errorf("create destination directory: %w", err)
		}
		if err := os.Rename(path, destination); err != nil {
			rollback()
			return fmt.Errorf("install %s: %w", relative, err)
		}
		installed = append(installed, destination)
	}
	for _, relative := range transaction.deletes {
		destination, err := safeProjectPath(transaction.root, relative)
		if err != nil {
			rollback()
			return fmt.Errorf("revalidate delete destination %s: %w", relative, err)
		}
		if _, statErr := os.Stat(destination); os.IsNotExist(statErr) {
			continue
		} else if statErr != nil {
			rollback()
			return fmt.Errorf("inspect delete destination %s: %w", relative, statErr)
		}
		backup := filepath.Join(transaction.backup, relative)
		if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
			rollback()
			return fmt.Errorf("create transaction delete backup directory: %w", err)
		}
		if err := os.Rename(destination, backup); err != nil {
			rollback()
			return fmt.Errorf("backup deleted file %s: %w", relative, err)
		}
		backedUp = append(backedUp, backup)
	}
	transaction.close()
	return nil
}

// Rollback discards staged files and closes the transaction.
func (transaction *Transaction) Rollback() error {
	if transaction == nil || !transaction.open {
		return nil
	}
	transaction.close()
	return nil
}

func (transaction *Transaction) close() {
	if !transaction.open {
		return
	}
	transaction.open = false
	_ = os.RemoveAll(transaction.stage)
	if transaction.backup != "" {
		_ = os.RemoveAll(transaction.backup)
	}
}
