//go:build !windows

package app

import (
	"os"
	"path/filepath"
	"runtime"
)

func defaultDataDir() (string, error) {
	if runtime.GOOS == "darwin" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(base, "CONTROL"), nil
	}
	base := os.Getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(base) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "CONTROL"), nil
}
