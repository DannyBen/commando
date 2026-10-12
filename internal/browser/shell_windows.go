package browser

import (
	"fmt"
	"golang.org/x/sys/windows"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	coCreateInstance   = windows.NewLazySystemDLL("ole32.dll").NewProc("CoCreateInstance")
	createShellItem    = windows.NewLazySystemDLL("shell32.dll").NewProc("SHCreateItemFromParsingName")
	clsidFileOperation = windows.GUID{Data1: 0x3ad05575, Data2: 0x8857, Data3: 0x4850, Data4: [8]byte{0x92, 0x77, 0x11, 0xb8, 0x5b, 0xdb, 0x8e, 0x09}}
	iidFileOperation   = windows.GUID{Data1: 0x947aab5f, Data2: 0x0a5c, Data3: 0x4c13, Data4: [8]byte{0xb4, 0xd6, 0x4b, 0xf7, 0x83, 0x6f, 0xc9, 0xf8}}
	iidShellItem       = windows.GUID{Data1: 0x43826d1e, Data2: 0xe718, Data3: 0x42ee, Data4: [8]byte{0xbc, 0x55, 0xa1, 0xe2, 0x61, 0xc3, 0x7b, 0xfe}}
	iidProgressSink    = windows.GUID{Data1: 0x04b0f1a7, Data2: 0x9490, Data3: 0x44bc, Data4: [8]byte{0x96, 0xe1, 0x42, 0x96, 0xa3, 0x12, 0x52, 0xe2}}
	iidUnknown         = windows.GUID{Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
)

// A small COM boundary uses only the slots needed here. IShellItem has fewer
// slots, but shares the first three IUnknown methods, including Release.
type shellObject struct{ vtable *[23]uintptr }

//go:uintptrescapes
func (o *shellObject) call(slot int, args ...uintptr) uint32 {
	arguments := append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)
	hr, _, _ := syscall.SyscallN(o.vtable[slot], arguments...)
	runtime.KeepAlive(o)
	return uint32(hr)
}

func shellHRESULT(hr uint32) error {
	if int32(hr) >= 0 {
		return nil
	}
	if hr&0xffff0000 == 0x80070000 {
		return syscall.Errno(hr & 0xffff)
	}
	return shellFailure(hr)
}

type shellFailure uint32

func (e shellFailure) Error() string { return fmt.Sprintf("Windows shell error 0x%08X", uint32(e)) }

// Shell objects are used only on the worker's locked STA thread.
func shellItem(path string) (*shellObject, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	var item *shellObject
	hr, _, _ := createShellItem.Call(uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&iidShellItem)), uintptr(unsafe.Pointer(&item)))
	runtime.KeepAlive(name)
	return item, shellHRESULT(uint32(hr))
}

func shellItemPath(item *shellObject) (string, error) {
	if item == nil {
		return "", syscall.EINVAL
	}
	const sigdnFileSystemPath = 0x80058000
	var path *uint16
	if err := shellHRESULT(item.call(5, sigdnFileSystemPath, uintptr(unsafe.Pointer(&path)))); err != nil {
		return "", err
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(path))
	return windows.UTF16PtrToString(path), nil
}
