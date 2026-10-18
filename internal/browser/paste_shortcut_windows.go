package browser

import (
	"fmt"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	iidPersistStream = windows.GUID{Data1: 0x00000109, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	createLinkStream = windows.NewLazySystemDLL("ole32.dll").NewProc("CreateStreamOnHGlobal")
)

// Let Windows serialize the shell link, then use the shared exclusive file
// creator to publish it. IPersistFile::Save would overwrite existing files.
func shortcutData(target string) ([]byte, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err != nil && err != syscall.Errno(1) {
		return nil, err
	}
	defer windows.CoUninitialize()
	var link *shellObject
	hr, _, _ := coCreateInstance.Call(uintptr(unsafe.Pointer(&clsidShellLink)), 0, 1, uintptr(unsafe.Pointer(&iidShellLink)), uintptr(unsafe.Pointer(&link)))
	if err := shellHRESULT(uint32(hr)); err != nil {
		return nil, err
	}
	defer link.call(2)
	for _, field := range []struct {
		method int
		value  string
	}{{20, target}, {9, filepath.Dir(target)}} { // IShellLinkW::SetPath / SetWorkingDirectory.
		value, err := windows.UTF16PtrFromString(field.value)
		if err != nil {
			return nil, err
		}
		hr := link.call(field.method, uintptr(unsafe.Pointer(value)))
		runtime.KeepAlive(value)
		if err := shellHRESULT(hr); err != nil {
			return nil, err
		}
	}
	var persisted *shellObject
	if err := shellHRESULT(link.call(0, uintptr(unsafe.Pointer(&iidPersistStream)), uintptr(unsafe.Pointer(&persisted)))); err != nil {
		return nil, err
	}
	defer persisted.call(2)
	var stream *shellObject
	hr, _, _ = createLinkStream.Call(0, 1, uintptr(unsafe.Pointer(&stream)))
	if err := shellHRESULT(uint32(hr)); err != nil {
		return nil, err
	}
	defer stream.call(2)
	if err := shellHRESULT(persisted.call(6, uintptr(unsafe.Pointer(stream)), 1)); err != nil { // IPersistStream::Save.
		return nil, err
	}
	var length uint64
	if err := shellHRESULT(stream.call(5, 0, 1, uintptr(unsafe.Pointer(&length)))); err != nil { // IStream::Seek, current position.
		return nil, err
	}
	if length == 0 || length > 16<<20 {
		return nil, fmt.Errorf("invalid shortcut size: %d", length)
	}
	if err := shellHRESULT(stream.call(5, 0, 0, 0)); err != nil { // Seek to the beginning.
		return nil, err
	}
	data := make([]byte, int(length))
	var read uint32
	hr32 := stream.call(3, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), uintptr(unsafe.Pointer(&read)))
	runtime.KeepAlive(data)
	if err := shellHRESULT(hr32); err != nil {
		return nil, err
	}
	if int(read) != len(data) {
		return nil, fmt.Errorf("incomplete shortcut stream")
	}
	return data, nil
}
