//go:build !windows

package browser

import "io/fs"

// Unix dotfiles are a naming convention, separate from Windows attributes.
func setEntryAttributes(*Entry, fs.FileInfo) {}

func sizeTraversalLink(info fs.FileInfo) bool { return info.Mode()&fs.ModeSymlink != 0 }
