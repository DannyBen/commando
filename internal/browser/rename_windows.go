package browser

import "golang.org/x/sys/windows"

func renameExclusive(oldPath, newPath string) error {
	old, err := windows.UTF16PtrFromString(oldPath)
	if err != nil {
		return err
	}
	new, err := windows.UTF16PtrFromString(newPath)
	if err != nil {
		return err
	}
	return windows.MoveFile(old, new)
}
