package app

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func localCompletionPath(path string) bool {
	if strings.HasPrefix(path, `\\`) {
		return false
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	root, err := windows.UTF16PtrFromString(filepath.VolumeName(absolute) + `\`)
	return err == nil && windows.GetDriveType(root) != windows.DRIVE_REMOTE
}
