package browser

import (
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	clipboardOLE      = windows.NewLazySystemDLL("ole32.dll")
	iidShellItemArray = windows.GUID{Data1: 0xb63ea76d, Data2: 0x1f85, Data3: 0x456f, Data4: [8]byte{0xa1, 0x9c, 0x48, 0x15, 0x9e, 0xfa, 0x85, 0x8b}}
)

func shellClipboardAvailable() bool {
	name, _ := windows.UTF16PtrFromString("Shell IDList Array")
	format, _, _ := registerClipboardFormat.Call(uintptr(unsafe.Pointer(name)))
	runtime.KeepAlive(name)
	if format == 0 {
		return false
	}
	available, _, _ := clipboardUser32.NewProc("IsClipboardFormatAvailable").Call(format)
	return available != 0
}

// Acquire, enumerate and release the clipboard data on the transfer's OLE STA.
// No COM pointer crosses a thread boundary and no source is treated as a disk path.
func shellClipboardItems(sequence uint32) (_ *shellObject, names []string, err error) {
	if FileClipboardSequence() != sequence {
		return nil, nil, ErrClipboardChanged
	}
	var data, items *shellObject
	hr, _, _ := clipboardOLE.NewProc("OleGetClipboard").Call(uintptr(unsafe.Pointer(&data)))
	if err := shellHRESULT(uint32(hr)); err != nil {
		return nil, nil, err
	}
	if data == nil {
		return nil, nil, ErrNoClipboardFiles
	}
	defer data.call(2)
	hr, _, _ = clipboardShell32.NewProc("SHCreateShellItemArrayFromDataObject").Call(
		uintptr(unsafe.Pointer(data)), uintptr(unsafe.Pointer(&iidShellItemArray)), uintptr(unsafe.Pointer(&items)))
	if err := shellHRESULT(uint32(hr)); err != nil {
		return nil, nil, err
	}
	if items == nil {
		return nil, nil, ErrNoClipboardFiles
	}
	defer func() {
		if err != nil {
			items.call(2)
		}
	}()
	var count uint32
	if err := shellHRESULT(items.call(7, uintptr(unsafe.Pointer(&count)))); err != nil {
		return nil, nil, err
	}
	if count == 0 || count > 100_000 {
		return nil, nil, ErrNoClipboardFiles
	}
	for index := range count {
		var item *shellObject
		if err := shellHRESULT(items.call(8, uintptr(index), uintptr(unsafe.Pointer(&item)))); err != nil {
			return nil, nil, err
		}
		name, err := shellItemName(item, 0x80028000) // SIGDN_DESKTOPABSOLUTEPARSING.
		item.call(2)
		if err != nil {
			return nil, nil, err
		}
		if name == "" {
			return nil, nil, ErrNoClipboardFiles
		}
		names = append(names, name)
	}
	if FileClipboardSequence() != sequence {
		return nil, nil, ErrClipboardChanged
	}
	return items, names, nil
}
