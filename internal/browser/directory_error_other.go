//go:build !windows

package browser

import (
	"errors"
	"syscall"
)

func isDirectoryUnavailable(err error) bool { return errors.Is(err, syscall.ENOTDIR) }
