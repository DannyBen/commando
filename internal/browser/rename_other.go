//go:build !windows && !linux

package browser

import "errors"

func renameExclusive(oldPath, newPath string) error { return errors.ErrUnsupported }
