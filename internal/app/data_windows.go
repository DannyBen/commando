package app

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

func defaultDataDir() (string, error) {
	base, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, windows.KF_FLAG_DEFAULT)
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "CONTROL"), nil
}
