// Package fs provides safe staged file writes for gx generators.
package fs

import (
	"errors"
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
// symlink after Write/Delete validation cannot redirect the commit. If commit
// fails, rollback failures are joined with the original error instead of being
// hidden from the caller.
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
	rollback := func() error {
		var rollbackErrors []error
		for index := len(installed) - 1; index >= 0; index-- {
			relative, err := filepath.Rel(transaction.root, installed[index])
			if err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("resolve installed rollback path: %w", err))
				continue
			}
			destination, err := safeProjectPath(transaction.root, relative)
			if err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("revalidate installed rollback path %s: %w", relative, err))
				continue
			}
			if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("remove installed file %s: %w", relative, err))
			}
		}
		for index := len(backedUp) - 1; index >= 0; index-- {
			backupPath := backedUp[index]
			relative, err := filepath.Rel(transaction.backup, backupPath)
			if err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("resolve backup rollback path: %w", err))
				continue
			}
			destination, err := safeProjectPath(transaction.root, relative)
			if err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("revalidate backup rollback path %s: %w", relative, err))
				continue
			}
			if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("create rollback parent for %s: %w", relative, err))
				continue
			}
			if err := os.Rename(backupPath, destination); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("restore %s: %w", relative, err))
			}
		}
		return errors.Join(rollbackErrors...)
	}
	fail := func(cause error) error { return errors.Join(cause, rollback()) }

	for _, path := range files {
		relative, err := filepath.Rel(transaction.stage, path)
		if err != nil {
			return fail(fmt.Errorf("resolve staged file: %w", err))
		}
		destination, err := safeProjectPath(transaction.root, relative)
		if err != nil {
			return fail(fmt.Errorf("revalidate destination %s: %w", relative, err))
		}
		if _, statErr := os.Stat(destination); statErr == nil {
			backup := filepath.Join(transaction.backup, relative)
			if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
				return fail(fmt.Errorf("create transaction backup directory: %w", err))
			}
			if err := os.Rename(destination, backup); err != nil {
				return fail(fmt.Errorf("backup %s: %w", relative, err))
			}
			backedUp = append(backedUp, backup)
		} else if !os.IsNotExist(statErr) {
			return fail(fmt.Errorf("inspect destination %s: %w", relative, statErr))
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return fail(fmt.Errorf("create destination directory: %w", err))
		}
		destination, err = safeProjectPath(transaction.root, relative)
		if err != nil {
			return fail(fmt.Errorf("revalidate destination %s after parent creation: %w", relative, err))
		}
		if err := os.Rename(path, destination); err != nil {
			return fail(fmt.Errorf("install %s: %w", relative, err))
		}
		installed = append(installed, destination)
	}
	for _, relative := range transaction.deletes {
		destination, err := safeProjectPath(transaction.root, relative)
		if err != nil {
			return fail(fmt.Errorf("revalidate delete destination %s: %w", relative, err))
		}
		if _, statErr := os.Stat(destination); os.IsNotExist(statErr) {
			continue
		} else if statErr != nil {
			return fail(fmt.Errorf("inspect delete destination %s: %w", relative, statErr))
		}
		backup := filepath.Join(transaction.backup, relative)
		if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
			return fail(fmt.Errorf("create transaction delete backup directory: %w", err))
		}
		if err := os.Rename(destination, backup); err != nil {
			return fail(fmt.Errorf("backup deleted file %s: %w", relative, err))
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
	return transaction.close()
}

func (transaction *Transaction) close() error {
	if !transaction.open {
		return nil
	}
	transaction.open = false
	var cleanupErrors []error
	if err := os.RemoveAll(transaction.stage); err != nil {
		cleanupErrors = append(cleanupErrors, fmt.Errorf("remove transaction staging: %w", err))
	}
	if transaction.backup != "" {
		if err := os.RemoveAll(transaction.backup); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove transaction backup: %w", err))
		}
	}
	return errors.Join(cleanupErrors...)
}
