package fs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// DirectorySwap describes a staged directory and its project-relative target.
type DirectorySwap struct {
	Stage  string
	Target string
}

type directoryTarget struct {
	DirectorySwap
	backup    string
	hadTarget bool
	prepared  bool
	installed bool
}

// DirectoryTransaction atomically replaces a set of generated directories.
// Existing targets remain recoverable until Commit succeeds.
type DirectoryTransaction struct {
	backupRoot string
	targets    []directoryTarget
	done       bool
}

// BeginDirectoryTransaction swaps staged directories into the project and
// returns a transaction that can be committed or rolled back by the caller.
func BeginDirectoryTransaction(root string, swaps ...DirectorySwap) (*DirectoryTransaction, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve directory transaction root: %w", err)
	}
	targets := make(map[string]struct{}, len(swaps))
	for index, swap := range swaps {
		target := filepath.Clean(swap.Target)
		if _, err := safeProjectPath(root, target); err != nil {
			return nil, fmt.Errorf("invalid directory swap %d: %w", index, err)
		}
		for existing := range targets {
			if pathOverlaps(existing, target) {
				return nil, fmt.Errorf("directory target %q overlaps %q", swap.Target, existing)
			}
		}
		targets[target] = struct{}{}
		if filepath.IsAbs(swap.Stage) {
			info, statErr := os.Lstat(swap.Stage)
			if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return nil, fmt.Errorf("invalid directory stage %q", swap.Stage)
			}
		} else {
			return nil, fmt.Errorf("invalid directory stage %q", swap.Stage)
		}
	}
	backupRoot, err := os.MkdirTemp(root, ".gx-directory-backup-*")
	if err != nil {
		return nil, fmt.Errorf("create directory backup: %w", err)
	}
	transaction := &DirectoryTransaction{backupRoot: backupRoot, targets: make([]directoryTarget, 0, len(swaps))}
	for index, swap := range swaps {
		target := filepath.Clean(swap.Target)
		stage := filepath.Clean(swap.Stage)
		item := directoryTarget{DirectorySwap: DirectorySwap{Stage: stage, Target: filepath.Join(root, target)}, backup: filepath.Join(backupRoot, fmt.Sprintf("%d", index))}
		if _, statErr := os.Stat(item.Target); statErr == nil {
			if err := os.Rename(item.Target, item.backup); err != nil {
				_ = transaction.Rollback()
				return nil, fmt.Errorf("backup directory %s: %w", target, err)
			}
			item.hadTarget = true
		} else if !os.IsNotExist(statErr) {
			_ = transaction.Rollback()
			return nil, fmt.Errorf("inspect directory %s: %w", target, statErr)
		}
		item.prepared = true
		transaction.targets = append(transaction.targets, item)
	}
	for index := range transaction.targets {
		item := &transaction.targets[index]
		if err := os.MkdirAll(filepath.Dir(item.Target), 0o755); err != nil {
			_ = transaction.Rollback()
			return nil, fmt.Errorf("create directory parent: %w", err)
		}
		if err := os.Rename(item.Stage, item.Target); err != nil {
			_ = transaction.Rollback()
			return nil, fmt.Errorf("install directory %s: %w", item.Target, err)
		}
		item.installed = true
	}
	return transaction, nil
}

// Rollback restores all directories swapped by the transaction.
func (transaction *DirectoryTransaction) Rollback() error {
	if transaction == nil || transaction.done {
		return nil
	}
	transaction.done = true
	var rollbackErrors []error
	for index := len(transaction.targets) - 1; index >= 0; index-- {
		item := transaction.targets[index]
		if !item.prepared {
			continue
		}
		if item.installed {
			if err := os.RemoveAll(item.Target); err != nil {
				rollbackErrors = append(rollbackErrors, err)
				continue
			}
		}
		if item.hadTarget {
			if err := os.Rename(item.backup, item.Target); err != nil {
				rollbackErrors = append(rollbackErrors, err)
			}
		}
	}
	rollbackErrors = append(rollbackErrors, os.RemoveAll(transaction.backupRoot))
	return errors.Join(rollbackErrors...)
}

// Commit removes backups and makes the directory replacement permanent.
func (transaction *DirectoryTransaction) Commit() error {
	if transaction == nil || transaction.done {
		return nil
	}
	transaction.done = true
	if err := os.RemoveAll(transaction.backupRoot); err != nil {
		return fmt.Errorf("remove directory backup: %w", err)
	}
	return nil
}
