//go:build !windows

package app

// Non-Windows hosts support development; previews never initialize settings.
func systemPrefersDarkTheme() bool { return false }
