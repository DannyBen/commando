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

// Exercise the real shell only on disposable fixtures owned by this test.
func TestPermanentDeleteRemovesFileAndFolderThroughShell(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "folder"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"file.txt", filepath.Join("folder", "child.txt")} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("disposable fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Delete(root, []string{"file.txt", "folder"})
	if err != nil || len(result.Names) != 2 {
		t.Fatal("shell did not confirm both deletions", result, err)
	}
	for _, name := range result.Names {
		if _, err := os.Stat(filepath.Join(root, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("confirmed deletion left source behind", name, err)
		}
	}
}
