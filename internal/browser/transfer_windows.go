package browser

import (
	"errors"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	fileOpAdvise           = 3
	fileOpUnadvise         = 4
	fileOpOwner            = 9
	fileOpMoveItem         = 14
	fileOpCopyItem         = 16
	fofNoConfirmMkdir      = 0x0200
	fofRenameOnCollision   = 0x0008
	fofxPreserveExtensions = 0x00200000
)

// One locked STA worker runs the shell engine and its native progress/error
// dialogs. No Go copy, overwrite, source deletion or rollback follows it.
func transferNative(request TransferRequest, originals []string, trace *transferBatchTrace) (result TransferResult, err error) {
	result.Directory = request.Directory
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil && err != syscall.Errno(1) {
		return result, err
	}
	defer windows.CoUninitialize()
	var operation *shellObject
	hr, _, _ := coCreateInstance.Call(uintptr(unsafe.Pointer(&clsidFileOperation)), 0, 1, uintptr(unsafe.Pointer(&iidFileOperation)), uintptr(unsafe.Pointer(&operation)))
	if err := shellHRESULT(uint32(hr)); err != nil {
		return result, err
	}
	defer operation.call(2)
	flags := uint32(fofNoConfirmMkdir | fofNoConnectedElements)
	if !request.TemporarySources {
		flags |= fofxAddUndoRecord
	}
	if request.Duplicate {
		flags |= fofRenameOnCollision | fofxPreserveExtensions
	}
	// Leave progress, conflict and error UI enabled. Never answer Yes to All on
	// behalf of the user; Windows is the sole authority for transfer decisions.
	if err := shellHRESULT(operation.call(fileOpSetFlags, uintptr(flags))); err != nil {
		return result, err
	}
	if err := shellHRESULT(operation.call(fileOpOwner, request.Owner)); err != nil {
		return result, err
	}
	target, err := shellItem(request.Directory)
	if err != nil {
		return result, err
	}
	defer target.call(2)
	sink := newTransferSink(request)
	sink.trace = trace
	defer func() { sink.releaseSources(); sink.refs.Add(-1); runtime.KeepAlive(sink) }()
	var cookie uint32
	if err := shellHRESULT(operation.call(fileOpAdvise, uintptr(unsafe.Pointer(sink)), uintptr(unsafe.Pointer(&cookie)))); err != nil {
		return result, err
	}
	defer operation.call(fileOpUnadvise, uintptr(cookie))
	// Queue the complete selection before execution, with one shell dialog/batch.
	for i, path := range request.Paths {
		item, err := shellItem(path)
		if err != nil {
			return result, err
		}
		var name *uint16
		if len(request.Names) > 0 {
			name, err = windows.UTF16PtrFromString(request.Names[i])
			if err != nil {
				item.call(2)
				return result, err
			}
		}
		slot := fileOpCopyItem
		if request.Move {
			slot = fileOpMoveItem
		}
		trace.record("QueueItemBegin", map[string]any{"index": i, "source": path, "name": windows.UTF16PtrToString(name), "move": request.Move})
		queueHR := operation.call(slot, uintptr(unsafe.Pointer(item)), uintptr(unsafe.Pointer(target)), uintptr(unsafe.Pointer(name)), 0)
		trace.record("QueueItemReturned", map[string]any{"index": i, "hresult": traceHRESULT(queueHR)})
		err = shellHRESULT(queueHR)
		runtime.KeepAlive(name)
		item.call(2)
		if err != nil {
			return result, err
		}
	}
	trace.record("PerformOperationsBegin", map[string]any{"flags": flags})
	performHR := operation.call(fileOpPerform)
	trace.record("PerformOperationsReturned", traceHRESULT(performHR))
	performErr := shellHRESULT(performHR)
	var aborted int32
	abortHR := operation.call(fileOpAborted, uintptr(unsafe.Pointer(&aborted)))
	trace.record("GetAnyOperationsAborted", map[string]any{"aborted": aborted != 0, "hresult": traceHRESULT(abortHR)})
	abortErr := shellHRESULT(abortHR)
	wasAborted := aborted != 0 || shellTransferState(performHR) == transferCancelled
	observations, summary, observationErr := sink.snapshot(wasAborted || int32(performHR) < 0)
	result = transferResults(request, originals, observations, wasAborted, summary)
	// User cancellation is a normal partial outcome; its HRESULT is not a new
	// error dialog. The shell has already presented any operation errors.
	if result.Aborted && (errors.Is(performErr, windows.ERROR_CANCELLED) || errors.Is(performErr, shellFailure(eAbort)) || errors.Is(performErr, shellFailure(0x80270000)) || errors.Is(performErr, shellFailure(0x80270001))) {
		performErr = nil
	}
	return result, errors.Join(performErr, abortErr, observationErr)
}
