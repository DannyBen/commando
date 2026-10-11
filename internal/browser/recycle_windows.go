package browser

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	// IFileOperation slots, after IUnknown's three methods.
	fileOpSetFlags   = 5
	fileOpDeleteItem = 18
	fileOpPerform    = 21
	fileOpAborted    = 22

	fofSilent              = 0x0004
	fofNoConfirmation      = 0x0010
	fofNoErrorUI           = 0x0400
	fofNoConnectedElements = 0x2000
	fofNoRecurseReparse    = 0x8000
	fofxRecycleOnDelete    = 0x00080000
	fofxEarlyFailure       = 0x00100000
	fofxAddUndoRecord      = 0x20000000
	tsRecycle              = 0x0080

	eAbort       = 0x80004004
	eNoInterface = 0x80004002
	ePointer     = 0x80004003
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

// All COM calls and synchronous callbacks stay on an STA thread. The directory
// worker invokes this function; no shell work or dialogs run on the Gio thread.
func recyclePaths(paths []string) (int, error) {
	major, minor, _ := windows.RtlGetNtVersionNumbers()
	if major < 6 || major == 6 && minor < 2 {
		return 0, ErrRecycleUnavailable // Never substitute the legacy undo-if-possible API.
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil && err != syscall.Errno(1) {
		return 0, err // S_FALSE still requires matching CoUninitialize.
	}
	defer windows.CoUninitialize()
	for i, path := range paths {
		if err := recycleShellItem(path); err != nil {
			return i, err
		}
	}
	return len(paths), nil
}

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

func recycleShellItem(path string) error {
	var operation *shellObject
	hr, _, _ := coCreateInstance.Call(uintptr(unsafe.Pointer(&clsidFileOperation)), 0, 1,
		uintptr(unsafe.Pointer(&iidFileOperation)), uintptr(unsafe.Pointer(&operation)))
	if err := shellHRESULT(uint32(hr)); err != nil {
		return err
	}
	var sink *recycleSink
	defer func() {
		operation.call(2)
		if sink != nil {
			sink.refs.Add(-1)
		}
		runtime.KeepAlive(sink)
	}()
	// RECYCLEONDELETE requests recycling rather than destructive deletion.
	// A pre-delete sink adds a veto when the shell isn't planning recycling.
	// Disable shell forms/progress; existing contextual feedback owns results.
	flags := fofxRecycleOnDelete | fofxEarlyFailure | fofxAddUndoRecord |
		fofSilent | fofNoConfirmation | fofNoErrorUI | fofNoConnectedElements | fofNoRecurseReparse
	if err := shellHRESULT(operation.call(fileOpSetFlags, uintptr(flags))); err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	var item *shellObject
	hr, _, _ = createShellItem.Call(uintptr(unsafe.Pointer(name)), 0,
		uintptr(unsafe.Pointer(&iidShellItem)), uintptr(unsafe.Pointer(&item)))
	runtime.KeepAlive(name)
	if err := shellHRESULT(uint32(hr)); err != nil {
		return err
	}
	defer item.call(2)
	sink = &recycleSink{vtable: &recycleSinkVtable}
	sink.refs.Store(1)
	if err := shellHRESULT(operation.call(fileOpDeleteItem, uintptr(unsafe.Pointer(item)), uintptr(unsafe.Pointer(sink)))); err != nil {
		return err
	}
	performErr := shellHRESULT(operation.call(fileOpPerform))
	var aborted int32
	abortErr := shellHRESULT(operation.call(fileOpAborted, uintptr(unsafe.Pointer(&aborted))))
	return sink.result(performErr, abortErr, aborted != 0)
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

type recycleSink struct {
	vtable                       *[19]uintptr
	refs                         atomic.Int32
	mu                           sync.Mutex
	refused, completed, recycled bool
	err                          error
}

func (s *recycleSink) preDelete(flags uint32) uintptr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if flags&tsRecycle == 0 {
		s.refused = true
		return eAbort
	}
	return 0
}

func (s *recycleSink) postDelete(hr uint32, newItem uintptr) uintptr {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completed, s.err = true, shellHRESULT(hr)
	s.recycled = s.err == nil && newItem != 0
	if s.err != nil || !s.recycled {
		return eAbort
	}
	return 0
}

func (s *recycleSink) result(performErr, abortErr error, aborted bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.refused {
		return ErrRecycleUnavailable
	}
	if s.err != nil {
		return s.err
	}
	// A queue/Perform success alone does not establish that the item was recycled.
	if s.completed && s.recycled {
		return nil
	}
	if performErr != nil {
		return performErr
	}
	if abortErr != nil {
		return abortErr
	}
	if aborted {
		return ErrRecycleUnavailable
	}
	return ErrRecycleUnconfirmed
}

// Static callbacks avoid allocating a new, never-freed Windows callback for
// every operation. The complete IFileOperationProgressSink ABI is represented.
var recycleSinkVtable = [19]uintptr{
	syscall.NewCallback(func(this *recycleSink, iid *windows.GUID, out *unsafe.Pointer) uintptr {
		if out == nil || iid == nil {
			return ePointer
		}
		*out = nil
		guid := *iid
		if guid != iidUnknown && guid != iidProgressSink {
			return eNoInterface
		}
		*out = unsafe.Pointer(this)
		this.refs.Add(1)
		return 0
	}),
	syscall.NewCallback(func(this *recycleSink) uintptr { return uintptr(this.refs.Add(1)) }),
	syscall.NewCallback(func(this *recycleSink) uintptr { return uintptr(this.refs.Add(-1)) }),
	syscall.NewCallback(func(this *recycleSink) uintptr { return 0 }),                                 // StartOperations
	syscall.NewCallback(func(this *recycleSink, hr uintptr) uintptr { return 0 }),                     // FinishOperations
	syscall.NewCallback(func(this *recycleSink, flags, item, name uintptr) uintptr { return eAbort }), // PreRename
	syscall.NewCallback(func(this *recycleSink, flags, item, name, hr, created uintptr) uintptr { return eAbort }),
	syscall.NewCallback(func(this *recycleSink, flags, item, dest, name uintptr) uintptr { return eAbort }), // PreMove
	syscall.NewCallback(func(this *recycleSink, flags, item, dest, name, hr, created uintptr) uintptr { return eAbort }),
	syscall.NewCallback(func(this *recycleSink, flags, item, dest, name uintptr) uintptr { return eAbort }), // PreCopy
	syscall.NewCallback(func(this *recycleSink, flags, item, dest, name, hr, created uintptr) uintptr { return eAbort }),
	syscall.NewCallback(func(this *recycleSink, flags, item uintptr) uintptr {
		return this.preDelete(uint32(flags))
	}),
	syscall.NewCallback(func(this *recycleSink, flags, item, hr, created uintptr) uintptr {
		return this.postDelete(uint32(hr), created)
	}),
	syscall.NewCallback(func(this *recycleSink, flags, dest, name uintptr) uintptr { return eAbort }), // PreNew
	syscall.NewCallback(func(this *recycleSink, flags, dest, name, template, attrs, hr, created uintptr) uintptr {
		return eAbort
	}),
	syscall.NewCallback(func(this *recycleSink, total, soFar uintptr) uintptr { return 0 }),
	syscall.NewCallback(func(this *recycleSink) uintptr { return 0 }), // ResetTimer
	syscall.NewCallback(func(this *recycleSink) uintptr { return 0 }), // PauseTimer
	syscall.NewCallback(func(this *recycleSink) uintptr { return 0 }), // ResumeTimer
}
