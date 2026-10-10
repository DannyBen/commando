package browser

import "golang.org/x/sys/windows"

func ReadDrives() (Directory, error) {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return Directory{}, err
	}
	return Directory{Drives: true, Entries: driveEntries(mask)}, nil
}
