package browser

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var archiveReplaceFile = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReplaceFileW")

// ReplaceFile preserves the existing file's ACLs, creation time and named
// streams. Keep a backup during the call: some Windows failure modes can move
// the original away before failing to install the replacement.
func replaceArchiveFile(temporary, original string) error {
	backup, err := os.CreateTemp(filepath.Dir(original), ".commando-zip-backup-*")
	if err != nil {
		return err
	}
	if err := backup.Close(); err != nil {
		os.Remove(backup.Name())
		return err
	}
	if err := os.Remove(backup.Name()); err != nil {
		return err
	}
	from, err := windows.UTF16PtrFromString(temporary)
	if err != nil {
		os.Remove(backup.Name())
		return err
	}
	to, err := windows.UTF16PtrFromString(original)
	if err != nil {
		os.Remove(backup.Name())
		return err
	}
	saved, err := windows.UTF16PtrFromString(backup.Name())
	if err != nil {
		os.Remove(backup.Name())
		return err
	}
	ok, _, callErr := archiveReplaceFile.Call(uintptr(unsafe.Pointer(to)), uintptr(unsafe.Pointer(from)), uintptr(unsafe.Pointer(saved)), 0, 0, 0)
	if ok != 0 {
		os.Remove(backup.Name())
		return nil
	}
	info, backupErr := os.Stat(backup.Name())
	if os.IsNotExist(backupErr) {
		return callErr
	}
	if backupErr == nil && info.Size() == 0 {
		os.Remove(backup.Name()) // Replacement never populated the reserved backup.
		return callErr
	}
	if _, err := os.Stat(original); os.IsNotExist(err) && backupErr == nil {
		if err := renameExclusive(backup.Name(), original); err == nil {
			return callErr
		}
	}
	// Do not delete the only surviving original on a failed recovery. The full
	// error retains its path for diagnostics; the footer reports a short reason.
	return fmt.Errorf("%w: %v; backup: %s", ErrArchiveRecovery, callErr, backup.Name())
}
