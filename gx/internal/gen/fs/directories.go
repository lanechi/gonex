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
	relative  string
	backup    string
	hadTarget bool
	prepared  bool
	installed bool
}

// DirectoryTransaction atomically replaces a set of generated directories.
// Existing targets remain recoverable until Commit succeeds.
type DirectoryTransaction struct {
	root       string
	backupRoot string
	targets    []directoryTarget
	done       bool
}

// BeginDirectoryTransaction swaps staged directories into the project and
// returns a transaction that can be committed or rolled back by the caller.
// Every destination is revalidated immediately before each filesystem mutation
// so a parent changed into a symlink after initial validation cannot redirect
// generated output outside the project root.
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
	transaction := &DirectoryTransaction{root: root, backupRoot: backupRoot, targets: make([]directoryTarget, 0, len(swaps))}
	fail := func(cause error) (*DirectoryTransaction, error) {
		return nil, errors.Join(cause, transaction.Rollback())
	}

	for index, swap := range swaps {
		relative := filepath.Clean(swap.Target)
		stage := filepath.Clean(swap.Stage)
		destination, err := safeProjectPath(root, relative)
		if err != nil {
			return fail(fmt.Errorf("revalidate directory target %s before backup: %w", relative, err))
		}
		item := directoryTarget{
			DirectorySwap: DirectorySwap{Stage: stage, Target: destination},
			relative:      relative,
			backup:        filepath.Join(backupRoot, fmt.Sprintf("%d", index)),
		}
		if _, statErr := os.Stat(destination); statErr == nil {
			if err := os.Rename(destination, item.backup); err != nil {
				return fail(fmt.Errorf("backup directory %s: %w", relative, err))
			}
			item.hadTarget = true
		} else if !os.IsNotExist(statErr) {
			return fail(fmt.Errorf("inspect directory %s: %w", relative, statErr))
		}
		item.prepared = true
		transaction.targets = append(transaction.targets, item)
	}
	for index := range transaction.targets {
		item := &transaction.targets[index]
		destination, err := safeProjectPath(root, item.relative)
		if err != nil {
			return fail(fmt.Errorf("revalidate directory target %s before install: %w", item.relative, err))
		}
		item.Target = destination
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return fail(fmt.Errorf("create directory parent: %w", err))
		}
		// MkdirAll may have created parents; validate the complete chain again
		// immediately before the rename that publishes generated output.
		destination, err = safeProjectPath(root, item.relative)
		if err != nil {
			return fail(fmt.Errorf("revalidate directory target %s after parent creation: %w", item.relative, err))
		}
		item.Target = destination
		if err := os.Rename(item.Stage, destination); err != nil {
			return fail(fmt.Errorf("install directory %s: %w", destination, err))
		}
		item.installed = true
	}
	return transaction, nil
}

// Rollback restores all directories swapped by the transaction. If a target
// can no longer be proven safe, its backup is retained instead of being
// deleted so recovery remains possible.
func (transaction *DirectoryTransaction) Rollback() error {
	if transaction == nil || transaction.done {
		return nil
	}
	transaction.done = true
	var rollbackErrors []error
	retainBackup := false
	for index := len(transaction.targets) - 1; index >= 0; index-- {
		item := transaction.targets[index]
		if !item.prepared {
			continue
		}
		destination, err := safeProjectPath(transaction.root, item.relative)
		if err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("revalidate rollback target %s: %w", item.relative, err))
			retainBackup = retainBackup || item.hadTarget
			continue
		}
		if item.installed {
			if err := os.RemoveAll(destination); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("remove installed directory %s: %w", item.relative, err))
				retainBackup = retainBackup || item.hadTarget
				continue
			}
		}
		if item.hadTarget {
			destination, err = safeProjectPath(transaction.root, item.relative)
			if err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("revalidate restore target %s: %w", item.relative, err))
				retainBackup = true
				continue
			}
			if err := os.Rename(item.backup, destination); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("restore directory %s: %w", item.relative, err))
				retainBackup = true
			}
		}
	}
	if retainBackup {
		rollbackErrors = append(rollbackErrors, fmt.Errorf("directory transaction backup retained at %s", transaction.backupRoot))
	} else if err := os.RemoveAll(transaction.backupRoot); err != nil {
		rollbackErrors = append(rollbackErrors, err)
	}
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
