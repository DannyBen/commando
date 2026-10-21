package browser

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// A Shell source with no filesystem display name, like an archive member.
type virtualClipboardItem struct {
	table   *[23]uintptr
	parsing *shellObject
}

var virtualClipboardTable = [23]uintptr{
	1: syscall.NewCallback(func(*virtualClipboardItem) uintptr { return 1 }),
	2: syscall.NewCallback(func(*virtualClipboardItem) uintptr { return 1 }),
	5: syscall.NewCallback(func(item *virtualClipboardItem, kind uintptr, out **uint16) uintptr {
		if kind != 0x80028000 {
			return 0x80004005 // No SIGDN_FILESYSPATH.
		}
		return uintptr(item.parsing.call(5, kind, uintptr(unsafe.Pointer(out))))
	}),
}

func TestWindowsVirtualClipboardSourceUsesParsingNameAndNativeOutcomes(t *testing.T) {
	if unsafe.Offsetof(transferSink{}.vtable) != 0 {
		t.Fatal("COM vtable must remain the first field")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil && err != syscall.Errno(1) {
		t.Fatal(err)
	}
	defer windows.CoUninitialize()
	sequence := FileClipboardSequence()
	if _, _, err := shellClipboardItems(sequence + 1); !errors.Is(err, ErrClipboardChanged) {
		t.Fatal("changed clipboard reached OLE", err)
	}
	directory := resolvedTempDir(t)
	destination := filepath.Join(directory, "actual-name.txt")
	if err := os.WriteFile(destination, []byte("copied"), 0600); err != nil {
		t.Fatal(err)
	}
	created, err := shellItem(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer created.call(2)
	// Wrap a real item so Shell allocates its display-name string normally,
	// while the source interface itself refuses filesystem-path requests.
	source := &virtualClipboardItem{table: &virtualClipboardTable, parsing: created}
	name, err := shellItemName(created, 0x80028000)
	if err != nil {
		t.Fatal(err)
	}
	var pin runtime.Pinner
	pin.Pin(source)
	defer pin.Unpin()
	item := (*shellObject)(unsafe.Pointer(source))
	if _, err := shellItemPath(item); err == nil {
		t.Fatal("fixture unexpectedly has a filesystem path")
	}
	for _, status := range []uint32{0, 0x00270005, 0x800704c7} {
		request := TransferRequest{ShellClipboard: &sequence, Paths: []string{name}, Directory: directory}
		sink := newTransferSink(request)
		sink.pre(item)
		sink.post(item, nil, created, nil, status)
		observations, summary, err := sink.snapshot(false)
		sink.releaseSources()
		if err != nil {
			t.Fatal(err)
		}
		result := transferResults(request, request.Paths, observations, summary.cancelled, summary)
		switch status {
		case 0:
			if summary.Completed != 1 || len(result.Items) != 1 || result.Items[0].Entry.Name != "actual-name.txt" || result.Items[0].Moved {
				t.Fatalf("virtual source lost actual copy destination: %+v", result)
			}
		case 0x00270005:
			if summary.Skipped != 1 || len(result.Items) != 0 {
				t.Fatalf("skipped source counted as copied: %+v", result)
			}
		default:
			if !result.Aborted || summary.Unfinished != 1 || len(result.Items) != 0 {
				t.Fatalf("cancelled source counted as copied: %+v", result)
			}
		}
	}
}
