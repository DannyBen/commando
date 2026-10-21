package browser

import (
	"io/fs"
	"syscall"
)

// Use metadata already obtained by the directory worker. For links, attributes
// describe the listed link itself, not the target followed for navigation.
func setEntryAttributes(item *Entry, info fs.FileInfo) {
	if data, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
		item.Hidden = data.FileAttributes&syscall.FILE_ATTRIBUTE_HIDDEN != 0
		item.System = data.FileAttributes&syscall.FILE_ATTRIBUTE_SYSTEM != 0
	}
}

func sizeTraversalLink(info fs.FileInfo) bool {
	if info.Mode()&fs.ModeSymlink != 0 {
		return true
	}
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return info.IsDir() && ok && data.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0
}
