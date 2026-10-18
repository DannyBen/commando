package browser

import (
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

// All COM calls and synchronous callbacks stay on an STA thread. The directory
// worker invokes this function; no shell work or dialogs run on the Gio thread.
func recyclePaths(paths []string) (int, error) {
	return removalPaths(paths, false)
}

func deletePaths(paths []string) (int, error) {
	return removalPaths(paths, true)
}

func removalPaths(paths []string, permanent bool) (int, error) {
	major, minor, _ := windows.RtlGetNtVersionNumbers()
	if major < 6 || major == 6 && minor < 2 {
		if permanent {
			return 0, ErrDeleteUnavailable
		}
		return 0, ErrRecycleUnavailable // Never substitute the legacy undo-if-possible API.
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil && err != syscall.Errno(1) {
		return 0, err // S_FALSE still requires matching CoUninitialize.
	}
	defer windows.CoUninitialize()
	for i, path := range paths {
		if err := removeShellItem(path, permanent); err != nil {
			return i, err
		}
	}
	return len(paths), nil
}

func removeShellItem(path string, permanent bool) error {
	var operation *shellObject
	hr, _, _ := coCreateInstance.Call(uintptr(unsafe.Pointer(&clsidFileOperation)), 0, 1,
		uintptr(unsafe.Pointer(&iidFileOperation)), uintptr(unsafe.Pointer(&operation)))
	if err := shellHRESULT(uint32(hr)); err != nil {
		return err
	}
	var sink *removalSink
	defer func() {
		operation.call(2)
		if sink != nil {
			sink.refs.Add(-1)
		}
		runtime.KeepAlive(sink)
	}()
	// Set flags explicitly: permanent deletion must never inherit shell undo
	// defaults. The sink vetoes an operation that disagrees with the chosen mode.
	flags := fofxEarlyFailure | fofSilent | fofNoConfirmation | fofNoErrorUI | fofNoConnectedElements | fofNoRecurseReparse
	if !permanent {
		flags |= fofxRecycleOnDelete | fofxAddUndoRecord
	}
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
	sink = &removalSink{vtable: &removalSinkVtable, permanent: permanent}
	sink.refs.Store(1)
	if err := shellHRESULT(operation.call(fileOpDeleteItem, uintptr(unsafe.Pointer(item)), uintptr(unsafe.Pointer(sink)))); err != nil {
		return err
	}
	performErr := shellHRESULT(operation.call(fileOpPerform))
	var aborted int32
	abortErr := shellHRESULT(operation.call(fileOpAborted, uintptr(unsafe.Pointer(&aborted))))
	return sink.result(performErr, abortErr, aborted != 0)
}

type removalSink struct {
	vtable                        *[19]uintptr
	refs                          atomic.Int32
	mu                            sync.Mutex
	refused, completed, confirmed bool
	permanent                     bool
	err                           error
}

func (s *removalSink) preDelete(flags uint32) uintptr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if (flags&tsRecycle != 0) == s.permanent {
		s.refused = true
		return eAbort
	}
	return 0
}

func (s *removalSink) postDelete(flags, hr uint32, newItem uintptr) uintptr {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.completed, s.err = true, shellHRESULT(hr)
	s.confirmed = hr == 0 && ((s.permanent && newItem == 0 && flags&tsRecycle == 0) || (!s.permanent && newItem != 0 && flags&tsRecycle != 0))
	if s.err != nil || !s.confirmed {
		return eAbort
	}
	return 0
}

func (s *removalSink) result(performErr, abortErr error, aborted bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	unavailable, unconfirmed := ErrRecycleUnavailable, ErrRecycleUnconfirmed
	if s.permanent {
		unavailable, unconfirmed = ErrDeleteUnavailable, ErrDeleteUnconfirmed
	}
	if s.refused {
		return unavailable
	}
	if s.err != nil {
		return s.err
	}
	// Queue/Perform success alone does not establish an item's outcome.
	if s.completed && s.confirmed {
		return nil
	}
	if performErr != nil {
		return performErr
	}
	if abortErr != nil {
		return abortErr
	}
	if aborted {
		return unavailable
	}
	return unconfirmed
}

// Static callbacks avoid allocating a new, never-freed Windows callback for
// every operation. The complete IFileOperationProgressSink ABI is represented.
var removalSinkVtable = [19]uintptr{
	syscall.NewCallback(func(this *removalSink, iid *windows.GUID, out *unsafe.Pointer) uintptr {
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
	syscall.NewCallback(func(this *removalSink) uintptr { return uintptr(this.refs.Add(1)) }),
	syscall.NewCallback(func(this *removalSink) uintptr { return uintptr(this.refs.Add(-1)) }),
	syscall.NewCallback(func(this *removalSink) uintptr { return 0 }),                                 // StartOperations
	syscall.NewCallback(func(this *removalSink, hr uintptr) uintptr { return 0 }),                     // FinishOperations
	syscall.NewCallback(func(this *removalSink, flags, item, name uintptr) uintptr { return eAbort }), // PreRename
	syscall.NewCallback(func(this *removalSink, flags, item, name, hr, created uintptr) uintptr { return eAbort }),
	syscall.NewCallback(func(this *removalSink, flags, item, dest, name uintptr) uintptr { return eAbort }), // PreMove
	syscall.NewCallback(func(this *removalSink, flags, item, dest, name, hr, created uintptr) uintptr { return eAbort }),
	syscall.NewCallback(func(this *removalSink, flags, item, dest, name uintptr) uintptr { return eAbort }), // PreCopy
	syscall.NewCallback(func(this *removalSink, flags, item, dest, name, hr, created uintptr) uintptr { return eAbort }),
	syscall.NewCallback(func(this *removalSink, flags, item uintptr) uintptr {
		return this.preDelete(uint32(flags))
	}),
	syscall.NewCallback(func(this *removalSink, flags, item, hr, created uintptr) uintptr {
		return this.postDelete(uint32(flags), uint32(hr), created)
	}),
	syscall.NewCallback(func(this *removalSink, flags, dest, name uintptr) uintptr { return eAbort }), // PreNew
	syscall.NewCallback(func(this *removalSink, flags, dest, name, template, attrs, hr, created uintptr) uintptr {
		return eAbort
	}),
	syscall.NewCallback(func(this *removalSink, total, soFar uintptr) uintptr { return 0 }),
	syscall.NewCallback(func(this *removalSink) uintptr { return 0 }), // ResetTimer
	syscall.NewCallback(func(this *removalSink) uintptr { return 0 }), // PauseTimer
	syscall.NewCallback(func(this *removalSink) uintptr { return 0 }), // ResumeTimer
}
