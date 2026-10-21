package browser

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type transferSource struct {
	path      string
	directory bool
	item      *shellObject
}

type transferSink struct {
	vtable      *[19]uintptr
	refs        atomic.Int32
	mu          sync.Mutex
	sources     map[uintptr]transferSource
	sourcePaths map[string]uintptr
	journal     transferJournal
	outcomes    transferOutcomes
	err         error
	trace       *transferBatchTrace

	shellClipboard bool
}

func newTransferSink(request TransferRequest) *transferSink {
	sink := &transferSink{vtable: &transferSinkVtable, shellClipboard: request.ShellClipboard != nil, sources: make(map[uintptr]transferSource), sourcePaths: make(map[string]uintptr), journal: newTransferJournal(request), outcomes: newTransferOutcomes(request)}
	sink.refs.Store(1)
	return sink
}

func (s *transferSink) pre(item *shellObject) uintptr {
	path, err := s.sourcePath(item)
	if err != nil {
		// Observing metadata must not preempt Windows permission/error UI.
		return 0
	}
	s.mu.Lock()
	needed := s.journal.needs(path)
	s.mu.Unlock()
	directory := false
	if needed && !s.shellClipboard {
		info, err := os.Lstat(path)
		directory = err == nil && info.IsDir()
	}
	s.mu.Lock()
	item.call(1)
	if key, ok := s.sourcePaths[transferPathKey(path)]; ok {
		s.sources[key].item.call(2)
		delete(s.sources, key)
	}
	s.sources[uintptr(unsafe.Pointer(item))] = transferSource{path: path, directory: directory, item: item}
	s.sourcePaths[transferPathKey(path)] = uintptr(unsafe.Pointer(item))
	s.mu.Unlock()
	return 0
}

func (s *transferSink) post(item, target, created *shellObject, name *uint16, hr uint32) uintptr {
	s.mu.Lock()
	source, ok := s.sources[uintptr(unsafe.Pointer(item))]
	delete(s.sources, uintptr(unsafe.Pointer(item)))
	if ok {
		delete(s.sourcePaths, transferPathKey(source.path))
	}
	s.mu.Unlock()
	if !ok {
		if path, err := s.sourcePath(item); err == nil {
			source.path = path
			s.mu.Lock()
			if key, exists := s.sourcePaths[transferPathKey(path)]; exists {
				source, ok = s.sources[key]
				delete(s.sources, key)
				delete(s.sourcePaths, transferPathKey(path))
			}
			s.mu.Unlock()
		}
	}
	if ok {
		defer source.item.call(2)
	}
	if !shellTransferCompleted(hr) {
		s.mu.Lock()
		s.outcomes.observe(source.path, hr)
		s.mu.Unlock()
		return 0 // Windows owns retries, skips, errors and cancellation.
	}
	// A completion status plus a resulting shell item confirms an action.
	// Positive skip/ignore/retry statuses are not completed transfers.
	if !ok {
		// Shell callbacks may supply another interface object for the same
		// entry. Its filesystem display name is sufficient for reconciliation.
		if source.path == "" {
			s.fail(ErrPathIdentity)
			return 0
		}
	}
	s.mu.Lock()
	needed := s.journal.needs(source.path)
	s.mu.Unlock()
	if !needed {
		s.mu.Lock()
		s.outcomes.observe(source.path, hr)
		s.mu.Unlock()
		return 0
	}
	destination := ""
	if created != nil {
		destination, _ = shellItemPath(created)
	}
	if destination == "" && target != nil {
		folder, err := shellItemPath(target)
		if err == nil {
			actualName := windows.UTF16PtrToString(name)
			if actualName == "" {
				actualName = filepath.Base(source.path)
			}
			destination = filepath.Join(folder, actualName)
		}
	}
	if destination == "" {
		s.fail(ErrPathIdentity)
		return 0
	}
	if info, err := os.Lstat(destination); err == nil {
		source.directory = info.IsDir()
	}
	s.mu.Lock()
	s.outcomes.observe(source.path, hr)
	s.journal.observe(transferObservation{source: source.path, destination: destination, directory: source.directory})
	s.mu.Unlock()
	return 0
}

func (s *transferSink) sourcePath(item *shellObject) (string, error) {
	if s.shellClipboard {
		return shellItemName(item, 0x80028000) // SIGDN_DESKTOPABSOLUTEPARSING.
	}
	return shellItemPath(item)
}

func (s *transferSink) fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err == nil {
		s.err = err
	}
}

func (s *transferSink) snapshot(interrupted bool) ([]transferObservation, TransferSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	summary := s.outcomes.summary(interrupted)
	var failure error
	if summary.failure != 0 {
		failure = shellHRESULT(summary.failure)
	}
	return s.journal.snapshot(), summary, errors.Join(s.err, failure)
}

func (s *transferSink) releaseSources() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, source := range s.sources {
		source.item.call(2)
		delete(s.sources, key)
	}
}

// Full IFileOperationProgressSink ABI. Callbacks are static, rather than
// allocating an unreclaimable Windows callback for every transfer.
var transferSinkVtable = [19]uintptr{
	syscall.NewCallback(func(this *transferSink, iid *windows.GUID, out *unsafe.Pointer) uintptr {
		if out == nil || iid == nil {
			return ePointer
		}
		*out = nil
		if *iid != iidUnknown && *iid != iidProgressSink {
			return eNoInterface
		}
		*out = unsafe.Pointer(this)
		this.refs.Add(1)
		return 0
	}),
	syscall.NewCallback(func(this *transferSink) uintptr { return uintptr(this.refs.Add(1)) }),
	syscall.NewCallback(func(this *transferSink) uintptr { return uintptr(this.refs.Add(-1)) }),
	syscall.NewCallback(func(this *transferSink) uintptr {
		this.trace.record("StartOperations", nil)
		return 0
	}),
	syscall.NewCallback(func(this *transferSink, hr uintptr) uintptr {
		this.trace.record("FinishOperations", traceHRESULT(uint32(hr)))
		return 0
	}),
	syscall.NewCallback(func(this *transferSink, flags uintptr, item *shellObject, name *uint16) uintptr {
		this.traceCallback("PreRenameItem", flags, item, nil, nil, name, nil)
		return 0
	}),
	syscall.NewCallback(func(this *transferSink, flags uintptr, item *shellObject, name *uint16, hr uintptr, created *shellObject) uintptr {
		status := uint32(hr)
		this.traceCallback("PostRenameItem", flags, item, nil, created, name, &status)
		return 0
	}),
	syscall.NewCallback(func(this *transferSink, flags uintptr, item, target *shellObject, name *uint16) uintptr {
		this.traceCallback("PreMoveItem", flags, item, target, nil, name, nil)
		return this.pre(item)
	}),
	syscall.NewCallback(func(this *transferSink, flags uintptr, item, target *shellObject, name *uint16, hr uintptr, created *shellObject) uintptr {
		status := uint32(hr)
		this.traceCallback("PostMoveItem", flags, item, target, created, name, &status)
		return this.post(item, target, created, name, uint32(hr))
	}),
	syscall.NewCallback(func(this *transferSink, flags uintptr, item, target *shellObject, name *uint16) uintptr {
		this.traceCallback("PreCopyItem", flags, item, target, nil, name, nil)
		return this.pre(item)
	}),
	syscall.NewCallback(func(this *transferSink, flags uintptr, item, target *shellObject, name *uint16, hr uintptr, created *shellObject) uintptr {
		status := uint32(hr)
		this.traceCallback("PostCopyItem", flags, item, target, created, name, &status)
		return this.post(item, target, created, name, uint32(hr))
	}),
	syscall.NewCallback(func(this *transferSink, flags uintptr, item *shellObject) uintptr {
		this.traceCallback("PreDeleteItem", flags, item, nil, nil, nil, nil)
		return 0
	}),
	syscall.NewCallback(func(this *transferSink, flags uintptr, item *shellObject, hr uintptr, created *shellObject) uintptr {
		status := uint32(hr)
		this.traceCallback("PostDeleteItem", flags, item, nil, created, nil, &status)
		return 0
	}),
	syscall.NewCallback(func(this *transferSink, flags uintptr, target *shellObject, name *uint16) uintptr {
		this.traceCallback("PreNewItem", flags, nil, target, nil, name, nil)
		return 0
	}),
	syscall.NewCallback(func(this *transferSink, flags uintptr, target *shellObject, name, template *uint16, attrs, hr uintptr, created *shellObject) uintptr {
		status := uint32(hr)
		this.traceCallback("PostNewItem", flags, nil, target, created, name, &status)
		return 0
	}),
	syscall.NewCallback(func(this *transferSink, total, soFar uintptr) uintptr {
		this.trace.record("UpdateProgress", map[string]any{"work_total": uint32(total), "work_so_far": uint32(soFar)})
		return 0
	}),
	syscall.NewCallback(func(this *transferSink) uintptr { this.trace.record("ResetTimer", nil); return 0 }),
	syscall.NewCallback(func(this *transferSink) uintptr { this.trace.record("PauseTimer", nil); return 0 }),
	syscall.NewCallback(func(this *transferSink) uintptr { this.trace.record("ResumeTimer", nil); return 0 }),
}

type tracedShellItem struct {
	Pointer string `json:"pointer"`
	Path    string `json:"path,omitempty"`
	Error   string `json:"error,omitempty"`
}

func traceShellItem(item *shellObject) *tracedShellItem {
	if item == nil {
		return nil
	}
	path, err := shellItemPath(item)
	return &tracedShellItem{Pointer: fmt.Sprintf("0x%X", uintptr(unsafe.Pointer(item))), Path: path, Error: traceError(err)}
}

func (s *transferSink) traceCallback(event string, flags uintptr, item, target, created *shellObject, name *uint16, status *uint32) {
	if s.trace == nil {
		return
	}
	s.mu.Lock()
	cached, known := s.sources[uintptr(unsafe.Pointer(item))]
	s.mu.Unlock()
	data := map[string]any{
		"flags":  fmt.Sprintf("0x%08X", uint32(flags)),
		"source": traceShellItem(item), "target": traceShellItem(target), "created": traceShellItem(created),
		"name": windows.UTF16PtrToString(name), "cached_source": known, "cached_source_path": cached.path,
	}
	if status != nil {
		data["hresult"] = traceHRESULT(*status)
	}
	s.trace.record(event, data)
}
