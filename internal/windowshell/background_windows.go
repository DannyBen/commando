package windowshell

import (
	"fmt"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func backgroundMenu(hwnd uintptr, paths []string, timings *MenuTimings) (*object, error) {
	start := time.Now()
	if len(paths) != 1 {
		return nil, fmt.Errorf("background menu requires one folder")
	}
	paths, err := siblingPaths(paths, true)
	if err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString(paths[0])
	if err != nil {
		return nil, err
	}
	var id unsafe.Pointer
	hr, _, _ := parseDisplayName.Call(uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&id)), 0, 0)
	runtime.KeepAlive(name)
	if id != nil {
		defer windows.CoTaskMemFree(id)
	}
	if err := hresult(hr); err != nil {
		return nil, fmt.Errorf("SHParseDisplayName: %w", err)
	}
	var folder *object
	hr, _, _ = bindToObject.Call(0, uintptr(id), 0, uintptr(unsafe.Pointer(&iidFolder)), uintptr(unsafe.Pointer(&folder)))
	if err := hresult(hr); err != nil {
		return nil, fmt.Errorf("SHBindToObject: %w", err)
	}
	defer folder.call(2)
	timings.Resolve = time.Since(start)
	acquire := time.Now()
	var menu *object
	// IShellFolder::CreateViewObject requests the folder's background menu.
	// GetUIObjectOf would instead request a menu for a contained item.
	hr = uintptr(folder.call(8, hwnd, uintptr(unsafe.Pointer(&iidMenu)), uintptr(unsafe.Pointer(&menu))))
	timings.Object = time.Since(acquire)
	if err := hresult(hr); err != nil {
		return nil, fmt.Errorf("CreateViewObject: %w", err)
	}
	return menu, nil
}
