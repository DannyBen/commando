package browser

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

func TestUnavailableWindowsDirectoryErrorsPermitRecovery(t *testing.T) {
	for _, err := range []error{windows.ERROR_DIRECTORY, windows.ERROR_INVALID_DRIVE, windows.ERROR_NOT_READY,
		windows.ERROR_DEV_NOT_EXIST, windows.ERROR_BAD_NETPATH, windows.ERROR_BAD_NET_NAME} {
		if !IsMissingDirectory(&os.PathError{Op: "read", Path: `Z:\missing`, Err: err}) {
			t.Fatalf("unavailable volume/share did not permit recovery: %v", err)
		}
	}
	for _, err := range []error{windows.ERROR_ACCESS_DENIED, windows.ERROR_SHARING_VIOLATION, windows.ERROR_INVALID_NAME} {
		if IsMissingDirectory(err) {
			t.Fatalf("unrelated Windows error permits recovery: %v", err)
		}
	}
}
