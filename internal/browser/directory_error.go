package browser

import (
	"errors"
	"io/fs"
)

// IsMissingDirectory includes file replacements and unavailable volumes/shares.
// Permission and other read failures must not navigate to a different location.
func IsMissingDirectory(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || isDirectoryUnavailable(err)
}
