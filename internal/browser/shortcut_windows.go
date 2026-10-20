package browser

import (
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	clsidShellLink            = windows.GUID{Data1: 0x00021401, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidShellLink              = windows.GUID{Data1: 0x000214f9, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidPersistFile            = windows.GUID{Data1: 0x0000010b, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	createShellItemFromIDList = windows.NewLazySystemDLL("shell32.dll").NewProc("SHCreateItemFromIDList")
)

// FolderShortcutTarget inspects a shortcut without invoking it or saving changes.
// File and virtual-shell targets return no folder, retaining native opening.
// Run on the directory worker; COM and filesystem access stay off the UI thread.
// https://learn.microsoft.com/en-us/windows/win32/shell/links
func FolderShortcutTarget(path string) (string, error) {
	return shortcutTarget(path, true)
}

// ShortcutTarget reads a saved link without resolving or probing its destination.
// Home can list offline/network favorites without touching their target folders.
func ShortcutTarget(path string) (string, error) {
	return shortcutTarget(path, false)
}

func shortcutTarget(path string, resolve bool) (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err != nil && err != syscall.Errno(1) {
		return "", err
	}
	defer windows.CoUninitialize()
	var link *shellObject
	hr, _, _ := coCreateInstance.Call(uintptr(unsafe.Pointer(&clsidShellLink)), 0, 1, uintptr(unsafe.Pointer(&iidShellLink)), uintptr(unsafe.Pointer(&link)))
	if err := shellHRESULT(uint32(hr)); err != nil {
		return "", err
	}
	defer link.call(2)
	var persisted *shellObject
	if err := shellHRESULT(link.call(0, uintptr(unsafe.Pointer(&iidPersistFile)), uintptr(unsafe.Pointer(&persisted)))); err != nil {
		return "", err
	}
	defer persisted.call(2)
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	// IPersistFile::Load(STGM_READ).
	hr32 := persisted.call(5, uintptr(unsafe.Pointer(name)), 0)
	runtime.KeepAlive(name)
	if err := shellHRESULT(hr32); err != nil {
		return "", err
	}
	// No dialogs, shortcut updates, recursive search, or distributed tracking.
	const noUI, noUpdate, noSearch, noTrack = 0x0001, 0x0008, 0x0010, 0x0020
	if resolve {
		if err := shellHRESULT(link.call(19, 0, noUI|noUpdate|noSearch|noTrack)); err != nil {
			return "", err
		}
	}
	// A PIDL -> IShellItem path avoids GetPath's fixed MAX_PATH buffer.
	var idList unsafe.Pointer
	if err := shellHRESULT(link.call(4, uintptr(unsafe.Pointer(&idList)))); err != nil {
		return "", err
	}
	if idList == nil {
		return "", nil
	}
	defer windows.CoTaskMemFree(idList)
	var target *shellObject
	hr, _, _ = createShellItemFromIDList.Call(uintptr(idList), uintptr(unsafe.Pointer(&iidShellItem)), uintptr(unsafe.Pointer(&target)))
	if err := shellHRESULT(uint32(hr)); err != nil {
		return "", err
	}
	defer target.call(2)
	// Virtual shell locations have no filesystem path.
	folder, err := shellItemPath(target)
	if err != nil {
		return "", nil
	}
	if !resolve {
		return folder, nil
	}
	info, err := os.Stat(folder)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", nil
	}
	return folder, nil
}
