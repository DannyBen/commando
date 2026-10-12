package browser

import (
	"errors"

	"golang.org/x/sys/windows"
)

func isDirectoryUnavailable(err error) bool {
	return errors.Is(err, windows.ERROR_DIRECTORY) || errors.Is(err, windows.ERROR_INVALID_DRIVE) ||
		errors.Is(err, windows.ERROR_NOT_READY) || errors.Is(err, windows.ERROR_DEV_NOT_EXIST) ||
		errors.Is(err, windows.ERROR_BAD_NETPATH) || errors.Is(err, windows.ERROR_BAD_NET_NAME)
}
