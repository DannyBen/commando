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
)

type RecycleResult struct {
	Directory string   // Resolved backing directory, for updating alias listings.
	Names     []string // Only entries confirmed to have reached the Recycle Bin.
}

// Recycle preflights all names and paths, then recycles in listing order and
// stops at the first failure. Previously recycled entries are reported even
// when a later item fails. There is no permanent-delete fallback.
func Recycle(directory string, names []string) (RecycleResult, error) {
	return recycle(directory, names, ResolveOperationPath, recyclePaths)
}

func recycle(directory string, names []string, resolve func(string) (string, error), perform func([]string) (int, error)) (RecycleResult, error) {
	var result RecycleResult
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
			return result, &os.PathError{Op: "recycle", Path: name, Err: err}
		}
	}
	count, err := perform(paths)
	result.Names = append([]string(nil), names[:count]...)
	if err != nil && count < len(names) {
		err = &os.PathError{Op: "recycle", Path: names[count], Err: err}
	}
	return result, err
}
