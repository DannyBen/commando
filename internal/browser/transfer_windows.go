package browser

import (
	"errors"
	"io/fs"
	"unsafe"

	"golang.org/x/sys/windows"
)

var copyFile = windows.NewLazySystemDLL("kernel32.dll").NewProc("CopyFileW")

func crossDevice(err error) bool { return errors.Is(err, windows.ERROR_NOT_SAME_DEVICE) }

func copyFileExclusive(source, destination string, _ fs.FileInfo) error {
	src, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	dst, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	ok, _, err := copyFile.Call(uintptr(unsafe.Pointer(src)), uintptr(unsafe.Pointer(dst)), 1)
	if ok == 0 {
		return err
	}
	return nil
}
