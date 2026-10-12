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
	// MoveFile may copy/delete regular files across volumes internally. Zero
	// flags keep this a rename: no replacement and no implicit cross-volume
	// copy. Transfers handle ERROR_NOT_SAME_DEVICE with our verified staging.
	return windows.MoveFileEx(old, new, 0)
}
