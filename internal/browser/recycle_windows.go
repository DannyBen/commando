package browser

import (
	"errors"
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

	fofNoConfirmation      = 0x0010
	fofNoConnectedElements = 0x2000
	fofNoRecurseReparse    = 0x8000
	fofxRecycleOnDelete    = 0x00080000
	fofxAddUndoRecord      = 0x20000000
	tsRecycle              = 0x0080

	eAbort       = 0x80004004
	eNoInterface = 0x80004002
	ePointer     = 0x80004003
)

// One STA operation owns the whole selection and its native recovery dialogs.
// Only the worker calls COM; the UI keeps consuming events while Windows runs.
func recyclePaths(paths []string, owner uintptr) (removalBatch, error) {
	return removalPaths(paths, false, owner)
}

func deletePaths(paths []string, owner uintptr) (removalBatch, error) {
	return removalPaths(paths, true, owner)
}

func removalPaths(paths []string, permanent bool, owner uintptr) (removalBatch, error) {
	return removalVerifiedPaths(paths, permanent, owner, nil)
}

func deleteVerifiedPaths(paths []string, owner uintptr, validate func(string) error) (removalBatch, error) {
	return removalVerifiedPaths(paths, true, owner, validate)
}

func removalVerifiedPaths(paths []string, permanent bool, owner uintptr, validate func(string) error) (batch removalBatch, err error) {
	trace := beginRemovalTrace(paths, permanent)
	defer func() {
		trace.record("RemovalResult", map[string]any{"completed": batch.completed, "error": traceError(err)})
	}()
	major, minor, _ := windows.RtlGetNtVersionNumbers()
	if major < 6 || major == 6 && minor < 2 {
		if permanent {
			return batch, ErrDeleteUnavailable
		}
		return batch, ErrRecycleUnavailable
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil && err != syscall.Errno(1) {
		return batch, err
	}
	defer windows.CoUninitialize()
	var operation *shellObject
	hr, _, _ := coCreateInstance.Call(uintptr(unsafe.Pointer(&clsidFileOperation)), 0, 1,
		uintptr(unsafe.Pointer(&iidFileOperation)), uintptr(unsafe.Pointer(&operation)))
	trace.record("RemovalCreateOperation", traceHRESULT(uint32(hr)))
	if err := shellHRESULT(uint32(hr)); err != nil {
		return batch, err
	}
	var sinks []*removalSink
	defer func() {
		operation.call(2)
		for _, sink := range sinks {
			sink.refs.Add(-1)
		}
		runtime.KeepAlive(sinks)
	}()
	// Commando owns the initial confirmation. Windows owns progress and error
	// recovery. Explicit flags and the pre-delete veto preserve the chosen mode.
	flags := fofNoConfirmation | fofNoConnectedElements | fofNoRecurseReparse
	if !permanent {
		flags |= fofxRecycleOnDelete | fofxAddUndoRecord
	}
	flagsHR := operation.call(fileOpSetFlags, uintptr(flags))
	trace.record("RemovalSetFlags", map[string]any{"flags": flags, "hresult": traceHRESULT(flagsHR)})
	if err := shellHRESULT(flagsHR); err != nil {
		return batch, err
	}
	if err := shellHRESULT(operation.call(fileOpOwner, owner)); err != nil {
		return batch, err
	}
	for _, path := range paths {
		item, err := shellItem(path)
		if err != nil {
			return batch, err
		}
		sink := &removalSink{vtable: &removalSinkVtable, permanent: permanent, trace: trace, validate: validate}
		sink.refs.Store(1)
		sinks = append(sinks, sink)
		queueHR := operation.call(fileOpDeleteItem, uintptr(unsafe.Pointer(item)), uintptr(unsafe.Pointer(sink)))
		item.call(2)
		trace.record("RemovalQueueItem", map[string]any{"path": path, "hresult": traceHRESULT(queueHR)})
		if err := shellHRESULT(queueHR); err != nil {
			return batch, err
		}
	}
	performHR := operation.call(fileOpPerform)
	trace.record("RemovalPerformOperations", traceHRESULT(performHR))
	performErr := shellHRESULT(performHR)
	var aborted int32
	abortHR := operation.call(fileOpAborted, uintptr(unsafe.Pointer(&aborted)))
	trace.record("RemovalOperationsAborted", map[string]any{"aborted": aborted != 0, "hresult": traceHRESULT(abortHR)})
	batch, err = finishRemoval(paths, sinks)
	if err != nil {
		return batch, err
	}
	// Windows has already explained failures. Skip/cancel is an incomplete
	// result, not a second generic error after the user dismisses its dialog.
	if shellTransferState(performHR) == transferCancelled {
		performErr = nil
	}
	return batch, errors.Join(performErr, shellHRESULT(abortHR))
}

func finishRemoval(paths []string, sinks []*removalSink) (removalBatch, error) {
	batch := removalBatch{completed: make([]bool, len(paths))}
	var refusal error
	for i, sink := range sinks {
		// Cancellation of a later entry must not discard earlier successes.
		// A child callback alone cannot confirm the selected root's removal.
		itemErr := sink.finish(paths[i], nil, nil, false)
		batch.completed[i] = itemErr == nil
		sink.trace.record("RemovalItemResult", map[string]any{"path": paths[i], "confirmed": itemErr == nil, "error": traceError(itemErr)})
		if sink.refused {
			refusal = itemErr
		}
	}
	return batch, refusal
}

type removalSink struct {
	// COM reads the vtable pointer from the first word of the object.
	vtable                        *[19]uintptr
	validate                      func(string) error
	refs                          atomic.Int32
	mu                            sync.Mutex
	refused, completed, confirmed bool
	approved                      bool
	permanent                     bool
	trace                         *transferBatchTrace
	err                           error
}

func (s *removalSink) preDelete(flags uint32) uintptr {
	return s.preDeleteItem(flags, nil)
}

func (s *removalSink) preDeleteItem(flags uint32, item *shellObject) uintptr {
	var validationErr error
	if s.validate != nil {
		var path string
		path, validationErr = shellItemPath(item)
		if validationErr == nil {
			validationErr = s.validate(path)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trace.record("RemovalPreDelete", map[string]any{"flags": flags, "permanent": s.permanent, "refused": (flags&tsRecycle != 0) == s.permanent})
	if (flags&tsRecycle != 0) == s.permanent {
		s.refused = true
		return eAbort
	}
	if validationErr != nil {
		s.refused, s.err = true, validationErr
		return eAbort
	}
	s.approved = true
	return 0
}

func (s *removalSink) postDelete(flags, hr uint32, newItem uintptr) uintptr {
	s.mu.Lock()
	defer s.mu.Unlock()
	// DONT_PROCESS_CHILDREN is a completed shell operation (for example, moving
	// a whole directory to the bin). NOT_HANDLED and PENDING remain informational.
	completed := hr == 0 || hr == 0x00270008
	if !completed && int32(hr) >= 0 {
		s.trace.record("RemovalPostDelete", map[string]any{"flags": flags, "hresult": traceHRESULT(hr), "has_new_item": newItem != 0, "confirmed": s.confirmed})
		return 0
	}
	s.completed, s.err = true, shellHRESULT(hr)
	s.confirmed = completed && ((s.permanent && newItem == 0 && flags&tsRecycle == 0) || (!s.permanent && newItem != 0 && flags&tsRecycle != 0))
	s.trace.record("RemovalPostDelete", map[string]any{"flags": flags, "hresult": traceHRESULT(hr), "has_new_item": newItem != 0, "confirmed": s.confirmed})
	// Let Windows resolve errors and skips without aborting the remaining batch.
	// The pre-delete callback remains the authority for vetoing a mode change.
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
		if s.err != nil {
			return s.err
		}
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

func (s *removalSink) finish(path string, performErr, abortErr error, aborted bool) error {
	result := s.result(performErr, abortErr, aborted)
	if !s.permanent {
		if result == nil {
			return confirmDeletedPath(path)
		}
		return result
	}
	if result != nil && result != ErrDeleteUnconfirmed {
		return result
	}
	// A successful child callback does not establish that the selected root
	// disappeared. Require native success and verify that root after traversal.
	if performErr != nil {
		return performErr
	}
	if abortErr != nil {
		return abortErr
	}
	if aborted {
		return ErrDeleteUnavailable
	}
	if !s.approved {
		return ErrDeleteUnconfirmed
	}
	err := confirmDeletedPath(path)
	s.trace.record("RemovalVerifyDeletedPath", map[string]any{"path": path, "error": traceError(err)})
	return err
}

func (s *removalSink) unexpected(event string, flags, hr uint32) uintptr {
	s.trace.record("RemovalUnexpectedCallback", map[string]any{"callback": event, "flags": flags, "hresult": traceHRESULT(hr)})
	return eAbort
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
	syscall.NewCallback(func(this *removalSink) uintptr { return 0 }),             // StartOperations
	syscall.NewCallback(func(this *removalSink, hr uintptr) uintptr { return 0 }), // FinishOperations
	syscall.NewCallback(func(this *removalSink, flags, item, name uintptr) uintptr {
		return this.unexpected("PreRename", uint32(flags), 0)
	}),
	syscall.NewCallback(func(this *removalSink, flags, item, name, hr, created uintptr) uintptr {
		return this.unexpected("PostRename", uint32(flags), uint32(hr))
	}),
	syscall.NewCallback(func(this *removalSink, flags, item, dest, name uintptr) uintptr {
		return this.unexpected("PreMove", uint32(flags), 0)
	}),
	syscall.NewCallback(func(this *removalSink, flags, item, dest, name, hr, created uintptr) uintptr {
		return this.unexpected("PostMove", uint32(flags), uint32(hr))
	}),
	syscall.NewCallback(func(this *removalSink, flags, item, dest, name uintptr) uintptr {
		return this.unexpected("PreCopy", uint32(flags), 0)
	}),
	syscall.NewCallback(func(this *removalSink, flags, item, dest, name, hr, created uintptr) uintptr {
		return this.unexpected("PostCopy", uint32(flags), uint32(hr))
	}),
	syscall.NewCallback(func(this *removalSink, flags uintptr, item *shellObject) uintptr {
		return this.preDeleteItem(uint32(flags), item)
	}),
	syscall.NewCallback(func(this *removalSink, flags, item, hr, created uintptr) uintptr {
		return this.postDelete(uint32(flags), uint32(hr), created)
	}),
	syscall.NewCallback(func(this *removalSink, flags, dest, name uintptr) uintptr {
		return this.unexpected("PreNew", uint32(flags), 0)
	}),
	syscall.NewCallback(func(this *removalSink, flags, dest, name, template, attrs, hr, created uintptr) uintptr {
		return this.unexpected("PostNew", uint32(flags), uint32(hr))
	}),
	syscall.NewCallback(func(this *removalSink, total, soFar uintptr) uintptr { return 0 }),
	syscall.NewCallback(func(this *removalSink) uintptr { return 0 }), // ResetTimer
	syscall.NewCallback(func(this *removalSink) uintptr { return 0 }), // PauseTimer
	syscall.NewCallback(func(this *removalSink) uintptr { return 0 }), // ResumeTimer
}
