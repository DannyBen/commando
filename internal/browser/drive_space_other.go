//go:build !windows

package browser

// Development builds do not probe host disks. Previews supply fixed samples.
func ReadFixedDriveSpace(string) (DriveSpace, bool) { return DriveSpace{}, false }
