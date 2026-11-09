package browser

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Capture CF_UNICODETEXT while the clipboard is locked. The filename editor
// must keep these contents even if another application replaces the clipboard.
func readClipboardTextLocked() string {
	handle, _, _ := getClipboardData.Call(13) // CF_UNICODETEXT
	if handle == 0 {
		return ""
	}
	size, _, _ := globalSize.Call(handle)
	if size < 2 || size > 1<<20 || size%2 != 0 {
		return ""
	}
	pointer, _, _ := globalLock.Call(handle)
	if pointer == 0 {
		return ""
	}
	defer globalUnlock.Call(handle)
	words := make([]uint16, int(size/2))
	moveClipboardMemory.Call(uintptr(unsafe.Pointer(&words[0])), pointer, size)
	return windows.UTF16ToString(words)
}
