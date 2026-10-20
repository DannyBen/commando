//go:build !windows

package browser

// Windows shell shortcuts retain ordinary desktop opening in development.
func FolderShortcutTarget(string) (string, error) { return "", nil }

func ShortcutTarget(string) (string, error) { return "", nil }
