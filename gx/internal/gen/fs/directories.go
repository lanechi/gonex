package fs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// DirectorySwap describes a staged directory and its project-relative target.
// Stage must be an absolute directory inside the same project root. Keeping
// stage and target under one retained os.Root lets publication use Root.Rename
// atomically without a path-string escape window.
type DirectorySwap struct {
	Stage  string
	Target string
}

type directoryTarget struct {
	DirectorySwap
	stageRelative string
	relative      string
	backup        string
	hadTarget     bool
	prepared      bool
	installed     bool
}

// DirectoryTransaction atomically replaces a set of generated directories.
// Existing targets remain recoverable until Commit succeeds.
type DirectoryTransaction struct {
	root       string
	rootHandle *os.Root
	backupRoot string
	targets    []directoryTarget
	done       bool
	committed  bool
}

// BeginDirectoryTransaction swaps staged directories into the project and
// returns a transaction that can be committed or rolled back by the caller.
// Every destination mutation is descriptor-relative to one retained os.Root;
// concurrent replacement of a path component cannot redirect output outside
// the project root.
func BeginDirectoryTransaction(root string, swaps ...DirectorySwap) (*DirectoryTransaction, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve directory transaction root: %w", err)
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open directory transaction root: %w", err)
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = rootHandle.Close()
		}
	}()

	targetNames := make(map[string]struct{}, len(swaps))
	preparedTargets := make([]directoryTarget, 0, len(swaps))
	for index, swap := range swaps {
		target, err := cleanProjectRelative(swap.Target)
		if err != nil {
			return nil, fmt.Errorf("invalid directory swap %d: %w", index, err)
		}
		if err := validateRootPath(rootHandle, target); err != nil {
			return nil, fmt.Errorf("invalid directory swap %d: %w", index, err)
		}
		for existing := range targetNames {
			if pathOverlaps(existing, target) {
				return nil, fmt.Errorf("directory target %q overlaps %q", swap.Target, existing)
			}
		}
		targetNames[target] = struct{}{}

		stageRelative, err := projectRelativePath(root, swap.Stage)
		if err != nil {
			return nil, fmt.Errorf("invalid directory stage %q: %w", swap.Stage, err)
		}
		if err := validateRootPath(rootHandle, stageRelative); err != nil {
			return nil, fmt.Errorf("invalid directory stage %q: %w", swap.Stage, err)
		}
		info, err := rootHandle.Lstat(stageRelative)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return nil, fmt.Errorf("invalid directory stage %q", swap.Stage)
		}
		preparedTargets = append(preparedTargets, directoryTarget{
			DirectorySwap:  DirectorySwap{Stage: swap.Stage, Target: swap.Target},
			stageRelative: stageRelative,
			relative:      target,
		})
	}

	// Validate the complete publication graph before the first rename. A stage
	// inside any target can be moved away when that target is backed up; nested
	// stages can similarly disappear when an earlier stage is installed. These
	// relationships are therefore invalid even when the target paths themselves
	// do not overlap.
	for left := range preparedTargets {
		for right := range preparedTargets {
			if pathOverlaps(preparedTargets[left].stageRelative, preparedTargets[right].relative) {
				return nil, fmt.Errorf(
					"directory stage %q overlaps target %q",
					preparedTargets[left].DirectorySwap.Stage,
					preparedTargets[right].DirectorySwap.Target,
				)
			}
			if left < right && pathOverlaps(preparedTargets[left].stageRelative, preparedTargets[right].stageRelative) {
				return nil, fmt.Errorf(
					"directory stage %q overlaps stage %q",
					preparedTargets[left].DirectorySwap.Stage,
					preparedTargets[right].DirectorySwap.Stage,
				)
			}
		}
	}

	backupRoot, err := createRootTempDir(rootHandle, ".gx-directory-backup-")
	if err != nil {
		return nil, fmt.Errorf("create directory backup: %w", err)
	}
	transaction := &DirectoryTransaction{
		root: root, rootHandle: rootHandle, backupRoot: backupRoot,
		targets: preparedTargets,
	}
	closeOnError = false
	fail := func(cause error) (*DirectoryTransaction, error) {
		return nil, errors.Join(cause, transaction.Rollback())
	}

	for index := range transaction.targets {
		item := &transaction.targets[index]
		if err := validateRootPath(rootHandle, item.relative); err != nil {
			return fail(fmt.Errorf("revalidate directory target %s before backup: %w", item.relative, err))
		}
		item.backup = filepath.Join(backupRoot, fmt.Sprintf("%d", index))
		if info, statErr := rootHandle.Lstat(item.relative); statErr == nil {
			if !info.IsDir() {
				return fail(fmt.Errorf("directory target %s is not a directory", item.relative))
			}
			if err := validateRootPath(rootHandle, item.relative); err != nil {
				return fail(fmt.Errorf("revalidate directory target %s before backup: %w", item.relative, err))
			}
			if err := rootHandle.Rename(item.relative, item.backup); err != nil {
				return fail(fmt.Errorf("backup directory %s: %w", item.relative, err))
			}
			item.hadTarget = true
		} else if !os.IsNotExist(statErr) {
			return fail(fmt.Errorf("inspect directory %s: %w", item.relative, statErr))
		}
		item.prepared = true
	}

	for index := range transaction.targets {
		item := &transaction.targets[index]
		if err := rootHandle.MkdirAll(filepath.Dir(item.relative), 0o755); err != nil {
			return fail(fmt.Errorf("create directory parent: %w", err))
		}
		if err := validateRootPath(rootHandle, item.relative); err != nil {
			return fail(fmt.Errorf("revalidate directory target %s before install: %w", item.relative, err))
		}
		if err := validateRootPath(rootHandle, item.stageRelative); err != nil {
			return fail(fmt.Errorf("revalidate directory stage %s before install: %w", item.stageRelative, err))
		}
		if err := rootHandle.Rename(item.stageRelative, item.relative); err != nil {
			return fail(fmt.Errorf("install directory %s: %w", item.relative, err))
		}
		item.installed = true
	}
	return transaction, nil
}

// Rollback restores all directories swapped by the transaction. If a target
// can no longer satisfy the generator's no-symlink rule, its backup is retained
// instead of being deleted so recovery remains possible.
func (transaction *DirectoryTransaction) Rollback() error {
	if transaction == nil || transaction.done || transaction.committed {
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
		if err := validateRootPath(transaction.rootHandle, item.relative); err != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("revalidate rollback target %s: %w", item.relative, err))
			retainBackup = retainBackup || item.hadTarget
			continue
		}
		if item.installed {
			if err := transaction.rootHandle.RemoveAll(item.relative); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("remove installed directory %s: %w", item.relative, err))
				retainBackup = retainBackup || item.hadTarget
				continue
			}
		}
		if item.hadTarget {
			if err := transaction.rootHandle.MkdirAll(filepath.Dir(item.relative), 0o755); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("create restore parent %s: %w", item.relative, err))
				retainBackup = true
				continue
			}
			if err := transaction.rootHandle.Rename(item.backup, item.relative); err != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("restore directory %s: %w", item.relative, err))
				retainBackup = true
			}
		}
	}
	if retainBackup {
		rollbackErrors = append(rollbackErrors, fmt.Errorf("directory transaction backup retained under project root at %s", transaction.backupRoot))
	} else if transaction.backupRoot != "" {
		if err := transaction.rootHandle.RemoveAll(transaction.backupRoot); err != nil {
			rollbackErrors = append(rollbackErrors, err)
		}
	}
	if err := transaction.rootHandle.Close(); err != nil {
		rollbackErrors = append(rollbackErrors, fmt.Errorf("close directory transaction root: %w", err))
	}
	transaction.rootHandle = nil
	return errors.Join(rollbackErrors...)
}

// Commit removes backups and makes the directory replacement permanent.
func (transaction *DirectoryTransaction) Commit() error {
	if transaction == nil || transaction.done {
		return nil
	}
	transaction.done = true
	transaction.committed = true
	var cleanupErrors []error
	if transaction.backupRoot != "" {
		if err := transaction.rootHandle.RemoveAll(transaction.backupRoot); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove directory backup: %w", err))
		}
	}
	if err := transaction.rootHandle.Close(); err != nil {
		cleanupErrors = append(cleanupErrors, fmt.Errorf("close directory transaction root: %w", err))
	}
	transaction.rootHandle = nil
	return errors.Join(cleanupErrors...)
}

// Committed reports whether publication completed before any cleanup error.
func (transaction *DirectoryTransaction) Committed() bool {
	return transaction != nil && transaction.committed
}
