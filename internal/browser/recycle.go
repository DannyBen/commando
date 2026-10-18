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
	Directory string   // Resolved backing directory, for updating alias listings.
	Names     []string // Only entries confirmed removed by the requested operation.
}

// Recycle preflights all names and paths, then recycles in listing order and
// stops at the first failure. Previously recycled entries are reported even
// when a later item fails. There is no permanent-delete fallback.
func Recycle(directory string, names []string) (RemovalResult, error) {
	return recycle(directory, names, ResolveOperationPath, recyclePaths)
}

// Delete permanently removes entries through the native shell. It shares the
// whole-selection preflight and partial-result contract with recycling.
func Delete(directory string, names []string) (RemovalResult, error) {
	return removeSelection(directory, names, "delete", ResolveOperationPath, deletePaths)
}

func recycle(directory string, names []string, resolve func(string) (string, error), perform func([]string) (int, error)) (RemovalResult, error) {
	return removeSelection(directory, names, "recycle", resolve, perform)
}

func removeSelection(directory string, names []string, operation string, resolve func(string) (string, error), perform func([]string) (int, error)) (RemovalResult, error) {
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
	count, err := perform(paths)
	result.Names = append([]string(nil), names[:count]...)
	if err != nil && count < len(names) {
		err = &os.PathError{Op: operation, Path: names[count], Err: err}
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
