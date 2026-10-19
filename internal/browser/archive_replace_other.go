//go:build !windows

package browser

import "os"

func replaceArchiveFile(temporary, original string) error { return os.Rename(temporary, original) }
