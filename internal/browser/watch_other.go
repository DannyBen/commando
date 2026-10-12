//go:build !windows

package browser

import (
	"context"
	"errors"
)

// Directory notifications are a Windows feature; development uses Ctrl+R.
func WatchDirectory(context.Context, string, func()) error { return errors.ErrUnsupported }
