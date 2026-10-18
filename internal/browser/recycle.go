package browser

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrRecycleUnavailable = errors.New("Recycle Bin unavailable")
	ErrRecycleUnconfirmed = errors.New("recycling was not confirmed")
	ErrDeleteUnavailable  = errors.New("permanent deletion unavailable")
	ErrDeleteUnconfirmed  = errors.New("deletion was not confirmed")
)

type RemovalResult struct {
	Directory  string   // Resolved backing directory, for updating alias listings.
	Names      []string // Only entries confirmed removed by the requested operation.
	Incomplete bool     // Windows skipped or cancelled part of the selection.
}

type removalBatch struct {
	completed []bool // Indexed by the original selection; skips need not be a suffix.
}

// Recycle preflights the whole selection, then lets Windows handle its batch.
// Only confirmed outcomes are reported. There is no permanent-delete fallback.
func Recycle(directory string, names []string, owner uintptr) (RemovalResult, error) {
	return recycle(directory, names, ResolveOperationPath, func(paths []string) (removalBatch, error) { return recyclePaths(paths, owner) })
}

// Delete permanently removes entries through the native shell. It shares the
// whole-selection preflight and partial-result contract with recycling.
func Delete(directory string, names []string, owner uintptr) (RemovalResult, error) {
	return removeSelection(directory, names, "delete", ResolveOperationPath, func(paths []string) (removalBatch, error) { return deletePaths(paths, owner) })
}

func recycle(directory string, names []string, resolve func(string) (string, error), perform func([]string) (removalBatch, error)) (RemovalResult, error) {
	return removeSelection(directory, names, "recycle", resolve, perform)
}

func removeSelection(directory string, names []string, operation string, resolve func(string) (string, error), perform func([]string) (removalBatch, error)) (RemovalResult, error) {
	var result RemovalResult
	if len(names) == 0 {
		return result, nil
	}
	seen := make(map[string]bool)
	for _, name := range names {
		if err := ValidateName(name); err != nil {
			return result, err
		}
		if seen[strings.ToUpper(name)] {
			return result, ErrInvalidName
		}
		seen[strings.ToUpper(name)] = true
	}
	root, err := resolve(directory)
	if err != nil {
		return result, err
	}
	result.Directory = filepath.Clean(root)
	paths := make([]string, len(names))
	for i, name := range names {
		paths[i], err = resolve(filepath.Join(directory, name))
		if err != nil {
			return result, &os.PathError{Op: operation, Path: name, Err: err}
		}
	}
	batch, err := perform(paths)
	failed := ""
	for i, name := range names {
		if i < len(batch.completed) && batch.completed[i] {
			result.Names = append(result.Names, name)
		} else if failed == "" {
			failed = name
		}
	}
	result.Incomplete = len(result.Names) != len(names)
	if err != nil && failed != "" {
		err = &os.PathError{Op: operation, Path: failed, Err: err}
	}
	return result, err
}

// Recursive shell deletion can finish without a final root callback. Verify the
// captured backing path after the native engine succeeds; never delete anything
// here. Lstat keeps a surviving broken symlink from looking like a removed entry.
func confirmDeletedPath(path string) error {
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return ErrDeleteUnconfirmed
}
