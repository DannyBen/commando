package browser

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Called under the existing clipboard lock. Copy owned bytes before releasing
// it: the clipboard can change while the user names the captured image.
func readClipboardImageLocked() ([]byte, bool, error) {
	name, _ := windows.UTF16PtrFromString("PNG")
	pngFormat, _, _ := registerClipboardFormat.Call(uintptr(unsafe.Pointer(name)))
	var imageErr error
	for _, format := range []uintptr{pngFormat, 17, 8} { // PNG, CF_DIBV5, CF_DIB
		if format == 0 {
			continue
		}
		handle, _, _ := getClipboardData.Call(format)
		if handle == 0 {
			continue
		}
		size, _, _ := globalSize.Call(handle)
		if size == 0 || size > maxClipboardImageBytes {
			imageErr = ErrClipboardImage
			continue
		}
		pointer, _, _ := globalLock.Call(handle)
		if pointer == 0 {
			imageErr = ErrClipboardImage
			continue
		}
		data := make([]byte, int(size))
		moveClipboardMemory.Call(uintptr(unsafe.Pointer(&data[0])), pointer, size)
		globalUnlock.Call(handle)
		return data, format != pngFormat, nil
	}
	if imageErr != nil {
		return nil, false, imageErr
	}
	return nil, false, ErrNoClipboardFiles
}
