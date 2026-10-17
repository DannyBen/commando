package browser

import (
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var shellExecuteEx = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

// Layout from SHELLEXECUTEINFOW; pointer fields follow the Windows ABI.
// https://learn.microsoft.com/en-us/windows/win32/api/shellapi/ns-shellapi-shellexecuteinfow
type shellExecuteInfo struct {
	size, mask                        uint32
	window                            windows.Handle
	verb, file, parameters, directory *uint16
	show                              int32
	instance                          windows.Handle
	idList                            unsafe.Pointer
	class                             *uint16
	classKey                          windows.Handle
	hotKey                            uint32
	icon, process                     windows.Handle
}

// OpenFile uses the associated Windows application, suppressing shell error UI.
// Called off the UI thread; the caller owns failure feedback.
func OpenFile(path string) error {
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	directory, err := windows.UTF16PtrFromString(filepath.Dir(path))
	if err != nil {
		return err
	}
	verb, _ := windows.UTF16PtrFromString("open")
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// S_FALSE (1) also means COM was successfully initialized on this thread.
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err != nil && err != syscall.Errno(1) {
		return err
	}
	defer windows.CoUninitialize()
	const noAsync, noUI = 0x100, 0x400
	info := shellExecuteInfo{mask: noAsync | noUI, verb: verb, file: file, directory: directory, show: windows.SW_SHOWNORMAL}
	info.size = uint32(unsafe.Sizeof(info))
	result, _, err := shellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(info)
	if result == 0 {
		return err
	}
	return nil
}
