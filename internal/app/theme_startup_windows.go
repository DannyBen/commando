package app

import "golang.org/x/sys/windows/registry"

// AppsUseLightTheme is the application preference, separate from Windows chrome.
// Missing, unreadable or unsupported preferences fall back to the light palette.
func systemPrefersDarkTheme() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	value, kind, err := key.GetIntegerValue("AppsUseLightTheme")
	return err == nil && kind == registry.DWORD && value == 0
}
