//go:build !windows

package browser

import (
	"os"
	"path/filepath"
)

func resolveOperationPath(path string) (string, error) {
	return filepath.Abs(path)
}

func verifyOperationPath(path, resolved string) error {
	source, err := os.Lstat(path)
	if err != nil {
		return err
	}
	target, err := os.Lstat(resolved)
	if err != nil {
		return err
	}
	if !os.SameFile(source, target) {
		return ErrPathIdentity
	}
	return nil
}
