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

// Callback tests exercise the native ABI without deleting files.
func TestRecycleSinkVetoesPermanentDelete(t *testing.T) {
	sink := &removalSink{vtable: &removalSinkVtable}
	hr, _, _ := syscall.SyscallN(sink.vtable[11], uintptr(unsafe.Pointer(sink)), 0, 0)
	if uint32(hr) != eAbort || !errors.Is(sink.result(nil, nil, false), ErrRecycleUnavailable) {
		t.Fatal("non-recycle operation was not vetoed")
	}
	runtime.KeepAlive(sink)
}

func TestRecycleSinkRequiresConfirmedBinItem(t *testing.T) {
	for _, tc := range []struct {
		hr   uint32
		item uintptr
		want error
	}{
		{0, 0, ErrRecycleUnconfirmed}, {0, 1, nil}, {0x80070005, 0, syscall.ERROR_ACCESS_DENIED},
	} {
		sink := &removalSink{vtable: &removalSinkVtable}
		hr, _, _ := syscall.SyscallN(sink.vtable[11], uintptr(unsafe.Pointer(sink)), tsRecycle, 0)
		if hr != 0 {
			t.Fatal("recycle request was vetoed")
		}
		syscall.SyscallN(sink.vtable[12], uintptr(unsafe.Pointer(sink)), tsRecycle, 0, uintptr(tc.hr), tc.item)
		if err := sink.result(nil, nil, false); !errors.Is(err, tc.want) {
			t.Fatalf("result=%v, want %v", err, tc.want)
		}
		runtime.KeepAlive(sink)
	}
	if err := new(removalSink).result(nil, nil, false); !errors.Is(err, ErrRecycleUnconfirmed) {
		t.Fatal("a queue success alone was reported as recycled")
	}
}

func TestRecycleSinkQueryInterfaceAndReferences(t *testing.T) {
	sink := &removalSink{vtable: &removalSinkVtable}
	sink.refs.Store(1)
	for _, iid := range []windows.GUID{iidUnknown, iidProgressSink, iidShellItem} {
		var out uintptr
		hr, _, _ := syscall.SyscallN(sink.vtable[0], uintptr(unsafe.Pointer(sink)), uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&out)))
		if iid == iidShellItem {
			if uint32(hr) != eNoInterface || out != 0 {
				t.Fatal("unsupported interface accepted")
			}
		} else {
			if hr != 0 || out != uintptr(unsafe.Pointer(sink)) || sink.refs.Load() != 2 {
				t.Fatal("interface/reference result incorrect")
			}
			syscall.SyscallN(sink.vtable[2], out)
		}
	}
	if sink.refs.Load() != 1 {
		t.Fatal("references unbalanced")
	}
	runtime.KeepAlive(sink)
}

func TestPermanentDeleteSinkVetoesRecyclingAndRequiresActualCompletion(t *testing.T) {
	for _, tc := range []struct {
		flags, hr uint32
		item      uintptr
		want      error
	}{
		{0, 0, 0, nil},
		{tsRecycle, 0, 1, ErrDeleteUnavailable},
		{0, 0, 1, ErrDeleteUnconfirmed},
		{0, 0x00270005, 0, ErrDeleteUnconfirmed}, // Shell skipped the item.
		{0, 0x80070005, 0, syscall.ERROR_ACCESS_DENIED},
	} {
		sink := &removalSink{vtable: &removalSinkVtable, permanent: true}
		pre, _, _ := syscall.SyscallN(sink.vtable[11], uintptr(unsafe.Pointer(sink)), uintptr(tc.flags), 0)
		if pre == 0 {
			syscall.SyscallN(sink.vtable[12], uintptr(unsafe.Pointer(sink)), uintptr(tc.flags), 0, uintptr(tc.hr), tc.item)
		}
		if err := sink.result(nil, nil, false); !errors.Is(err, tc.want) {
			t.Fatalf("flags=%x hr=%x item=%d result=%v want=%v", tc.flags, tc.hr, tc.item, err, tc.want)
		}
		runtime.KeepAlive(sink)
	}
	if err := (&removalSink{permanent: true}).result(nil, nil, false); !errors.Is(err, ErrDeleteUnconfirmed) {
		t.Fatal("queue success was reported as deletion")
	}
}

func TestRemovalSinkLetsInformationalResultsReachFinalCompletion(t *testing.T) {
	for _, permanent := range []bool{false, true} {
		sink := &removalSink{vtable: &removalSinkVtable, permanent: permanent}
		flags, newItem, unconfirmed := uintptr(tsRecycle), uintptr(1), ErrRecycleUnconfirmed
		if permanent {
			flags, newItem, unconfirmed = 514, 0, ErrDeleteUnconfirmed
		}
		this := uintptr(unsafe.Pointer(sink))
		if hr, _, _ := syscall.SyscallN(sink.vtable[11], this, flags, 0); hr != 0 {
			t.Fatal("pre-delete was refused")
		}
		for _, status := range []uintptr{0x00270003, 0x0027000b, 0x00270010} {
			if hr, _, _ := syscall.SyscallN(sink.vtable[12], this, flags, 0, status, 0); hr != 0 {
				t.Fatalf("informational status %#x aborted deletion: %#x", status, hr)
			}
			if err := sink.result(nil, nil, false); !errors.Is(err, unconfirmed) {
				t.Fatal("informational status was mistaken for confirmed removal", status, err)
			}
		}
		if hr, _, _ := syscall.SyscallN(sink.vtable[12], this, flags, 0, 0, newItem); hr != 0 {
			t.Fatal("final completion was refused")
		}
		// A late informational notification must not erase a confirmed outcome.
		syscall.SyscallN(sink.vtable[12], this, flags, 0, 0x00270003, 0)
		if err := sink.result(nil, nil, false); err != nil {
			t.Fatal("confirmed completion was lost", err)
		}
		runtime.KeepAlive(sink)
	}
}

// Exercise the real shell only on disposable fixtures owned by this test.
func TestPermanentDeleteRemovesFileAndFolderThroughShell(t *testing.T) {
	root := t.TempDir()
	tracePath, stopTrace, err := EnableTransferTrace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer stopTrace()
	t.Cleanup(func() {
		if t.Failed() {
			data, err := os.ReadFile(tracePath)
			if err != nil {
				t.Log(err)
				return
			}
			t.Logf("Native removal trace:\n%s", data)
		}
	})
	if err := os.Mkdir(filepath.Join(root, "folder with spaces"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"file with spaces.txt", filepath.Join("folder with spaces", "child.txt")} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("disposable fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Delete(root, []string{"file with spaces.txt", "folder with spaces"})
	if err != nil || len(result.Names) != 2 {
		t.Fatal("shell did not confirm both deletions", result, err)
	}
	for _, name := range result.Names {
		if _, err := os.Stat(filepath.Join(root, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("confirmed deletion left source behind", name, err)
		}
	}
}

func TestRemovalSinkRecognizesWholeItemCompletion(t *testing.T) {
	// Replay the recycling result observed in the desktop trace: Windows moved
	// the item to the bin and reports DONT_PROCESS_CHILDREN rather than S_OK.
	sink := &removalSink{vtable: &removalSinkVtable}
	this := uintptr(unsafe.Pointer(sink))
	syscall.SyscallN(sink.vtable[11], this, 642, 0)
	if hr, _, _ := syscall.SyscallN(sink.vtable[12], this, 642, 0, 0x00270008, 1); hr != 0 {
		t.Fatal("completed recycling was aborted", hr)
	}
	if err := sink.finish("unused", nil, nil, false); err != nil {
		t.Fatal("bin item was not recognized as recycled", err)
	}
	runtime.KeepAlive(sink)
}

func TestRecursivePermanentDeleteVerifiesSelectedRoot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "selected folder")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	sink := &removalSink{vtable: &removalSinkVtable, permanent: true}
	this := uintptr(unsafe.Pointer(sink))
	// Replay a root NOT_HANDLED notification followed by completed child items.
	syscall.SyscallN(sink.vtable[11], this, 514, 0)
	if hr, _, _ := syscall.SyscallN(sink.vtable[12], this, 514, 0, 0x00270003, 0); hr != 0 {
		t.Fatal("root notification aborted traversal")
	}
	syscall.SyscallN(sink.vtable[11], this, 2, 0)
	if hr, _, _ := syscall.SyscallN(sink.vtable[12], this, 4098, 0, 0x00270008, 0); hr != 0 {
		t.Fatal("child completion aborted traversal")
	}
	if err := sink.finish(path, nil, nil, false); !errors.Is(err, ErrDeleteUnconfirmed) {
		t.Fatal("child completion falsely confirmed the surviving root", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := sink.finish(path, nil, nil, false); err != nil {
		t.Fatal("native success with absent root was not confirmed", err)
	}
	if err := sink.finish(path, os.ErrPermission, nil, false); !errors.Is(err, os.ErrPermission) {
		t.Fatal("missing path hid native failure", err)
	}
	if err := sink.finish(path, nil, nil, true); !errors.Is(err, ErrDeleteUnavailable) {
		t.Fatal("missing path hid cancellation", err)
	}
	runtime.KeepAlive(sink)
	if err := (&removalSink{permanent: true}).finish(path, nil, nil, false); !errors.Is(err, ErrDeleteUnconfirmed) {
		t.Fatal("missing path without an approved operation was called successful", err)
	}
}
