package browser

import (
	"bytes"
	"io/fs"
	"unsafe"

	"golang.org/x/sys/windows"
)

func archiveClipboardFormat() (uintptr, error) {
	name, _ := windows.UTF16PtrFromString("Commando.ArchiveSelection.v1")
	format, _, err := registerClipboardFormat.Call(uintptr(unsafe.Pointer(name)))
	if format == 0 {
		return 0, err
	}
	return format, nil
}

func readArchiveClipboardLocked() (*ArchiveClipboard, error) {
	format, err := archiveClipboardFormat()
	if err != nil {
		return nil, err
	}
	handle, _, _ := getClipboardData.Call(format)
	if handle == 0 {
		return nil, nil
	}
	size, _, _ := globalSize.Call(handle)
	if size == 0 || size > maxArchiveClipboardBytes {
		return nil, fs.ErrInvalid
	}
	pointer, _, _ := globalLock.Call(handle)
	if pointer == 0 {
		return nil, fs.ErrInvalid
	}
	data := make([]byte, int(size))
	moveClipboardMemory.Call(uintptr(unsafe.Pointer(&data[0])), pointer, size)
	globalUnlock.Call(handle)
	return decodeArchiveClipboard(bytes.TrimRight(data, "\x00"))
}

// Publish only our private references, never CF_HDROP paths or file contents.
func WriteArchiveClipboard(reference *ArchiveClipboard) (snapshot FileClipboard, err error) {
	err = withFileClipboard(true, func() error { snapshot, err = writeArchiveClipboardLocked(reference); return err })
	return
}

func writeArchiveClipboardLocked(reference *ArchiveClipboard) (FileClipboard, error) {
	if reference == nil {
		if ok, _, err := emptyClipboard.Call(); ok == 0 {
			return FileClipboard{}, err
		}
		return FileClipboard{Sequence: FileClipboardSequence()}, nil
	}
	data, err := encodeArchiveClipboard(reference)
	if err != nil {
		return FileClipboard{}, err
	}
	format, err := archiveClipboardFormat()
	if err != nil {
		return FileClipboard{}, err
	}
	handle, err := clipboardMemory(append(data, 0))
	if err != nil {
		return FileClipboard{}, err
	}
	defer func() {
		if handle != 0 {
			globalFree.Call(handle)
		}
	}()
	if ok, _, err := emptyClipboard.Call(); ok == 0 {
		return FileClipboard{}, err
	}
	if ok, _, err := setClipboardData.Call(format, handle); ok == 0 {
		return FileClipboard{}, err
	}
	handle = 0
	copied, err := decodeArchiveClipboard(data)
	return FileClipboard{Archive: copied, Move: reference.Move, Sequence: FileClipboardSequence()}, err
}
