//go:build !windows

package browser

import "io/fs"

// Unix dotfiles are a naming convention, separate from Windows attributes.
func setEntryAttributes(*Entry, fs.FileInfo) {}
