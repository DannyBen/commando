package browser

import (
	"errors"
	"math"
	"testing"

	"golang.org/x/sys/windows"
)

func TestReadFixedDriveSpaceFiltersBeforeProbing(t *testing.T) {
	for _, kind := range []uint32{windows.DRIVE_UNKNOWN, windows.DRIVE_NO_ROOT_DIR, windows.DRIVE_REMOVABLE, windows.DRIVE_REMOTE, windows.DRIVE_CDROM, windows.DRIVE_RAMDISK, windows.DRIVE_FIXED} {
		calls := 0
		got, known := readFixedDriveSpace(`C:\`, func(*uint16) uint32 { return kind }, func(_ *uint16, free, total, volumeFree *uint64) error {
			calls++
			if volumeFree != nil {
				t.Fatal("mixed caller quota and whole-volume space")
			}
			*free, *total = 10, 30
			return nil
		})
		if kind == windows.DRIVE_FIXED {
			if !known || calls != 1 || got != (DriveSpace{Free: 10, Total: 30}) {
				t.Fatal(got, known, calls)
			}
		} else if known || calls != 0 {
			t.Fatalf("probed excluded drive type %d", kind)
		}
	}
}

func TestReadFixedDriveSpaceFailuresStayUnknown(t *testing.T) {
	for _, tc := range []struct {
		free, total uint64
		err         error
	}{{10, 30, errors.New("unavailable")}, {40, 30, nil}, {0, math.MaxUint64, nil}} {
		got, known := readFixedDriveSpace(`C:\`, func(*uint16) uint32 { return windows.DRIVE_FIXED }, func(_ *uint16, free, total, _ *uint64) error { *free, *total = tc.free, tc.total; return tc.err })
		if known || got != (DriveSpace{}) {
			t.Fatal("failed query supplied metadata")
		}
	}
}
