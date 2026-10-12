package browser

import (
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
	skipped     int
	err         error
}

func newTransferSink(request TransferRequest) *transferSink {
	sink := &transferSink{vtable: &transferSinkVtable, sources: make(map[uintptr]transferSource), sourcePaths: make(map[string]uintptr), journal: newTransferJournal(request)}
	sink.refs.Store(1)
	return sink
}

func (s *transferSink) pre(item *shellObject) uintptr {
	path, err := shellItemPath(item)
	if err != nil {
		// Observing metadata must not preempt Windows permission/error UI.
		return 0
	}
	s.mu.Lock()
	needed := s.journal.needs(path)
	s.mu.Unlock()
	directory := false
	if needed {
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
		if path, err := shellItemPath(item); err == nil {
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
	// A completion status plus a resulting shell item confirms an action.
	// Positive skip/ignore/retry statuses are not completed transfers.
	if !shellTransferCompleted(hr) {
		s.mu.Lock()
		s.skipped++
		s.mu.Unlock()
		return 0 // Let Windows continue, retry or cancel according to its own UI.
	}
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
	s.journal.observe(transferObservation{source: source.path, destination: destination, directory: source.directory})
	s.mu.Unlock()
	return 0
}

func (s *transferSink) fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err == nil {
		s.err = err
	}
}

func (s *transferSink) snapshot() ([]transferObservation, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.journal.snapshot(), s.skipped, s.err
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
	syscall.NewCallback(func(this *transferSink) uintptr { return 0 }),             // StartOperations
	syscall.NewCallback(func(this *transferSink, hr uintptr) uintptr { return 0 }), // FinishOperations
	syscall.NewCallback(func(this *transferSink, flags, item, name uintptr) uintptr { return 0 }),
	syscall.NewCallback(func(this *transferSink, flags, item, name, hr, created uintptr) uintptr { return 0 }),
	syscall.NewCallback(func(this *transferSink, flags uintptr, item *shellObject, target, name uintptr) uintptr {
		return this.pre(item)
	}),
	syscall.NewCallback(func(this *transferSink, flags uintptr, item, target *shellObject, name *uint16, hr uintptr, created *shellObject) uintptr {
		return this.post(item, target, created, name, uint32(hr))
	}),
	syscall.NewCallback(func(this *transferSink, flags uintptr, item *shellObject, target, name uintptr) uintptr {
		return this.pre(item)
	}),
	syscall.NewCallback(func(this *transferSink, flags uintptr, item, target *shellObject, name *uint16, hr uintptr, created *shellObject) uintptr {
		return this.post(item, target, created, name, uint32(hr))
	}),
	syscall.NewCallback(func(this *transferSink, flags, item uintptr) uintptr { return 0 }),
	syscall.NewCallback(func(this *transferSink, flags, item, hr, created uintptr) uintptr { return 0 }),
	syscall.NewCallback(func(this *transferSink, flags, target, name uintptr) uintptr { return 0 }),
	syscall.NewCallback(func(this *transferSink, flags, target, name, template, attrs, hr, created uintptr) uintptr { return 0 }),
	syscall.NewCallback(func(this *transferSink, total, soFar uintptr) uintptr { return 0 }),
	syscall.NewCallback(func(this *transferSink) uintptr { return 0 }),
	syscall.NewCallback(func(this *transferSink) uintptr { return 0 }),
	syscall.NewCallback(func(this *transferSink) uintptr { return 0 }),
}
