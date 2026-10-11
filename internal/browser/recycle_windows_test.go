package browser

import (
	"errors"
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// These tests exercise the native callback ABI without deleting any files.
func TestRecycleSinkVetoesPermanentDelete(t *testing.T) {
	sink := &recycleSink{vtable: &recycleSinkVtable}
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
		sink := &recycleSink{vtable: &recycleSinkVtable}
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
	if err := new(recycleSink).result(nil, nil, false); !errors.Is(err, ErrRecycleUnconfirmed) {
		t.Fatal("a queue success alone was reported as recycled")
	}
}

func TestRecycleSinkQueryInterfaceAndReferences(t *testing.T) {
	sink := &recycleSink{vtable: &recycleSinkVtable}
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
