//go:build !windows

package browser

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"syscall"
)

func crossDevice(err error) bool { return errors.Is(err, syscall.EXDEV) }

func copyFileExclusive(source, destination string, info fs.FileInfo) error {
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(dst, src)
	err = errors.Join(copyErr, dst.Close())
	if err != nil {
		return err
	}
	return os.Chtimes(destination, info.ModTime(), info.ModTime())
}
