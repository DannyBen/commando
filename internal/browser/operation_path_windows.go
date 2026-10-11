package browser

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func resolveOperationPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return resolveDOSPath(absolute, queryDriveMapping)
}

func queryDriveMapping(drive string) (string, error) {
	name, err := windows.UTF16PtrFromString(drive)
	if err != nil {
		return "", err
	}
	for size := 256; size <= 65536; size *= 2 {
		buffer := make([]uint16, size)
		n, err := windows.QueryDosDevice(name, &buffer[0], uint32(len(buffer)))
		if errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
			continue
		}
		if err != nil {
			return "", err
		}
		// Ignore undeleted prior mappings after the first NUL terminator.
		return windows.UTF16ToString(buffer[:n]), nil
	}
	return "", windows.ERROR_INSUFFICIENT_BUFFER
}

// Open metadata handles with sharing enabled so files already open in an editor
// can be checked. OPEN_REPARSE_POINT compares the selected link itself, including
// a broken link, rather than its target. No contents are read or modified.
func verifyOperationPath(path, resolved string) error {
	open := func(path string) (windows.Handle, error) {
		name, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return windows.InvalidHandle, err
		}
		handle, err := windows.CreateFile(name, 0,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
			nil, windows.OPEN_EXISTING,
			windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if err != nil {
			return windows.InvalidHandle, &os.PathError{Op: "verify path", Path: path, Err: err}
		}
		return handle, nil
	}
	source, err := open(path)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(source)
	target, err := open(resolved)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(target)
	var sourceInfo, targetInfo windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(source, &sourceInfo); err != nil {
		return err
	}
	if err := windows.GetFileInformationByHandle(target, &targetInfo); err != nil {
		return err
	}
	if sourceInfo.VolumeSerialNumber != targetInfo.VolumeSerialNumber ||
		sourceInfo.FileIndexHigh != targetInfo.FileIndexHigh || sourceInfo.FileIndexLow != targetInfo.FileIndexLow {
		return ErrPathIdentity
	}
	return nil
}
