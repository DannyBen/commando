//go:build !windows

package browser

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

func archiveClipboardIdentity(_ *os.File, info fs.FileInfo) (string, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fs.ErrInvalid
	}
	return fmt.Sprintf("%x:%x", stat.Dev, stat.Ino), nil
}
