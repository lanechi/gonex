// Package fs provides safe staged file writes for gx generators.
package fs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Transaction stages generated files below Root and publishes them on Commit.
// Rollback removes the staging directory and leaves existing files untouched.
// All publication and rollback mutations are descriptor-relative to root so a
// concurrent path/symlink replacement cannot redirect them outside the project.
type Transaction struct {
	root       string
	rootHandle *os.Root
	stage      string
	backup     string
	deletes    []string
	writes     map[string]struct{}
	open       bool
}

type backupEntry struct {
	backup      string
	destination string
}

// Delete stages removal of a project-relative file. Existing content remains
// untouched until Commit, where it is backed up with every staged write.
func (transaction *Transaction) Delete(relative string) error {
	if transaction == nil || !transaction.open || transaction.rootHandle == nil {
		return fmt.Errorf("file transaction is closed")
	}
	clean, err := cleanProjectRelative(relative)
	if err != nil {
		return fmt.Errorf("transaction path %q: %w", relative, err)
	}
	if err := validateRootPath(transaction.rootHandle, clean); err != nil {
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
	if info, err := transaction.rootHandle.Lstat(clean); err == nil && info.IsDir() {
		return fmt.Errorf("transaction path %q is an existing directory", relative)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect transaction path %q: %w", relative, err)
	}
	transaction.deletes = append(transaction.deletes, clean)
	return nil
}

// NewTransaction creates a staging transaction for root. The open os.Root is
// retained for the full transaction lifetime, binding every mutation to the
// same project directory even if its process-visible path is renamed.
func NewTransaction(root string) (*Transaction, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve transaction root: %w", err)
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open transaction root: %w", err)
	}
	stage, err := createRootTempDir(rootHandle, ".gx-stage-")
	if err != nil {
		_ = rootHandle.Close()
		return nil, fmt.Errorf("create transaction staging: %w", err)
	}
	return &Transaction{
		root: root, rootHandle: rootHandle, stage: stage,
		open: true, writes: make(map[string]struct{}),
	}, nil
}

// Write stages content at a project-relative path. Absolute paths and path
// traversal are rejected so generators cannot write outside the project.
func (transaction *Transaction) Write(relative string, content []byte, permission os.FileMode) error {
	if transaction == nil || !transaction.open || transaction.rootHandle == nil {
		return fmt.Errorf("file transaction is closed")
	}
	clean, err := cleanProjectRelative(relative)
	if err != nil {
		return fmt.Errorf("transaction path %q: %w", relative, err)
	}
	if err := validateRootPath(transaction.rootHandle, clean); err != nil {
		return fmt.Errorf("transaction path %q: %w", relative, err)
	}
	if info, err := transaction.rootHandle.Lstat(clean); err == nil && info.IsDir() {
		return fmt.Errorf("transaction path %q is an existing directory", relative)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect transaction path %q: %w", relative, err)
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
	stagePath := filepath.Join(transaction.stage, clean)
	if err := transaction.rootHandle.MkdirAll(filepath.Dir(stagePath), 0o755); err != nil {
		return fmt.Errorf("create staging directory for %s: %w", relative, err)
	}
	if permission == 0 {
		permission = 0o644
	}
	file, err := transaction.rootHandle.OpenFile(stagePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, permission)
	if err != nil {
		return fmt.Errorf("stage %s: %w", relative, err)
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return fmt.Errorf("stage %s: %w", relative, err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("flush staged %s: %w", relative, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close staged %s: %w", relative, err)
	}
	transaction.writes[clean] = struct{}{}
	return nil
}

// Commit publishes all staged files and closes the transaction. Every target
// mutation is performed through the transaction's retained os.Root. Root.Rename
// resolves both old and new paths under that descriptor, eliminating the
// validation-to-rename escape window of path-string publication.
func (transaction *Transaction) Commit() error {
	if transaction == nil || !transaction.open || transaction.rootHandle == nil {
		return fmt.Errorf("file transaction is closed")
	}
	files := make([]string, 0, len(transaction.writes))
	for relative := range transaction.writes {
		files = append(files, relative)
	}
	sort.Strings(files)

	backup, err := createRootTempDir(transaction.rootHandle, ".gx-backup-")
	if err != nil {
		return fmt.Errorf("create transaction backup: %w", err)
	}
	transaction.backup = backup
	installed := make([]string, 0, len(files))
	backedUp := make([]backupEntry, 0, len(files)+len(transaction.deletes))

	rollback := func() error {
		var rollbackErrors []error
		for index := len(installed) - 1; index >= 0; index-- {
			relative := installed[index]
			if err := validateRootPath(transaction.rootHandle, relative); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("revalidate installed rollback path %s: %w", relative, err))
				continue
			}
			if err := transaction.rootHandle.Remove(relative); err != nil && !os.IsNotExist(err) {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("remove installed file %s: %w", relative, err))
			}
		}
		for index := len(backedUp) - 1; index >= 0; index-- {
			entry := backedUp[index]
			if err := validateRootPath(transaction.rootHandle, entry.destination); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("revalidate backup rollback path %s: %w", entry.destination, err))
				continue
			}
			if err := transaction.rootHandle.MkdirAll(filepath.Dir(entry.destination), 0o755); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("create rollback parent for %s: %w", entry.destination, err))
				continue
			}
			if err := transaction.rootHandle.Rename(entry.backup, entry.destination); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("restore %s: %w", entry.destination, err))
			}
		}
		return errors.Join(rollbackErrors...)
	}
	fail := func(cause error) error { return errors.Join(cause, rollback()) }

	for _, relative := range files {
		if err := validateRootPath(transaction.rootHandle, relative); err != nil {
			return fail(fmt.Errorf("revalidate destination %s: %w", relative, err))
		}
		if info, statErr := transaction.rootHandle.Lstat(relative); statErr == nil {
			if info.IsDir() {
				return fail(fmt.Errorf("destination %s became a directory", relative))
			}
			backupPath := filepath.Join(transaction.backup, relative)
			if err := transaction.rootHandle.MkdirAll(filepath.Dir(backupPath), 0o755); err != nil {
				return fail(fmt.Errorf("create transaction backup directory: %w", err))
			}
			if err := validateRootPath(transaction.rootHandle, relative); err != nil {
				return fail(fmt.Errorf("revalidate destination %s before backup: %w", relative, err))
			}
			if err := transaction.rootHandle.Rename(relative, backupPath); err != nil {
				return fail(fmt.Errorf("backup %s: %w", relative, err))
			}
			backedUp = append(backedUp, backupEntry{backup: backupPath, destination: relative})
		} else if !os.IsNotExist(statErr) {
			return fail(fmt.Errorf("inspect destination %s: %w", relative, statErr))
		}
		if err := transaction.rootHandle.MkdirAll(filepath.Dir(relative), 0o755); err != nil {
			return fail(fmt.Errorf("create destination directory: %w", err))
		}
		if err := validateRootPath(transaction.rootHandle, relative); err != nil {
			return fail(fmt.Errorf("revalidate destination %s after parent creation: %w", relative, err))
		}
		stagePath := filepath.Join(transaction.stage, relative)
		if err := validateRootPath(transaction.rootHandle, stagePath); err != nil {
			return fail(fmt.Errorf("revalidate staged file %s: %w", relative, err))
		}
		if err := transaction.rootHandle.Rename(stagePath, relative); err != nil {
			return fail(fmt.Errorf("install %s: %w", relative, err))
		}
		installed = append(installed, relative)
	}

	for _, relative := range transaction.deletes {
		if err := validateRootPath(transaction.rootHandle, relative); err != nil {
			return fail(fmt.Errorf("revalidate delete destination %s: %w", relative, err))
		}
		if info, statErr := transaction.rootHandle.Lstat(relative); os.IsNotExist(statErr) {
			continue
		} else if statErr != nil {
			return fail(fmt.Errorf("inspect delete destination %s: %w", relative, statErr))
		} else if info.IsDir() {
			return fail(fmt.Errorf("delete destination %s became a directory", relative))
		}
		backupPath := filepath.Join(transaction.backup, relative)
		if err := transaction.rootHandle.MkdirAll(filepath.Dir(backupPath), 0o755); err != nil {
			return fail(fmt.Errorf("create transaction delete backup directory: %w", err))
		}
		if err := validateRootPath(transaction.rootHandle, relative); err != nil {
			return fail(fmt.Errorf("revalidate delete destination %s before backup: %w", relative, err))
		}
		if err := transaction.rootHandle.Rename(relative, backupPath); err != nil {
			return fail(fmt.Errorf("backup deleted file %s: %w", relative, err))
		}
		backedUp = append(backedUp, backupEntry{backup: backupPath, destination: relative})
	}
	return transaction.close()
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
	if transaction.rootHandle != nil {
		if transaction.stage != "" {
			if err := transaction.rootHandle.RemoveAll(transaction.stage); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("remove transaction staging: %w", err))
			}
		}
		if transaction.backup != "" {
			if err := transaction.rootHandle.RemoveAll(transaction.backup); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("remove transaction backup: %w", err))
			}
		}
		if err := transaction.rootHandle.Close(); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("close transaction root: %w", err))
		}
		transaction.rootHandle = nil
	}
	return errors.Join(cleanupErrors...)
}
