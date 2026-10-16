package browser

import (
	"io/fs"
	"syscall"
	"time"
)

func creationTime(info fs.FileInfo) time.Time {
	if data, ok := info.Sys().(*syscall.Win32FileAttributeData); ok && (data.CreationTime.HighDateTime != 0 || data.CreationTime.LowDateTime != 0) {
		return time.Unix(0, data.CreationTime.Nanoseconds())
	}
	return time.Time{}
}
