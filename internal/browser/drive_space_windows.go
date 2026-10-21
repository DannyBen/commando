package browser

import (
	"math"

	"golang.org/x/sys/windows"
)

// ReadFixedDriveSpace deliberately avoids space queries on network, removable,
// optical and unknown drives. Unavailable metadata leaves the row blank.
func ReadFixedDriveSpace(root string) (DriveSpace, bool) {
	return readFixedDriveSpace(root, windows.GetDriveType, windows.GetDiskFreeSpaceEx)
}

func readFixedDriveSpace(root string, driveType func(*uint16) uint32, readSpace func(*uint16, *uint64, *uint64, *uint64) error) (DriveSpace, bool) {
	path, err := windows.UTF16PtrFromString(root)
	if err != nil || driveType(path) != windows.DRIVE_FIXED {
		return DriveSpace{}, false
	}
	var free, total uint64
	// Use caller-visible free and total consistently when per-user quotas apply.
	if err := readSpace(path, &free, &total, nil); err != nil || free > total || total > math.MaxInt64 {
		return DriveSpace{}, false
	}
	return DriveSpace{Free: int64(free), Total: int64(total)}, true
}
