package browser

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestWindowsTransferSinkABIRecordsOriginalBeforeMoveAndActualNewName(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil && err != syscall.Errno(1) {
		t.Fatal(err)
	}
	defer windows.CoUninitialize()
	root := resolvedTempDir(t)
	source, destination := filepath.Join(root, "original.txt"), filepath.Join(root, "actual.txt")
	if err := os.WriteFile(source, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	item, err := shellItem(source)
	if err != nil {
		t.Fatal(err)
	}
	defer item.call(2)
	sink := newTransferSink(TransferRequest{Paths: []string{source}})
	this, ptr := uintptr(unsafe.Pointer(sink)), uintptr(unsafe.Pointer(item))
	if hr, _, _ := syscall.SyscallN(sink.vtable[7], this, 0, ptr, 0, 0); hr != 0 {
		t.Fatalf("pre-move refused: %x", hr)
	}
	if err := os.Rename(source, destination); err != nil {
		t.Fatal(err)
	}
	created, err := shellItem(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer created.call(2)
	if hr, _, _ := syscall.SyscallN(sink.vtable[8], this, 0, ptr, 0, 0, 0, uintptr(unsafe.Pointer(created))); hr != 0 {
		t.Fatalf("post-move failed: %x", hr)
	}
	observations, skipped, err := sink.snapshot()
	if err != nil || skipped != 0 || len(observations) != 1 || observations[0].source != source || observations[0].destination != destination {
		t.Fatalf("callback result: %+v %d %v", observations, skipped, err)
	}
	// A positive ignored status with a non-null target still isn't completion.
	syscall.SyscallN(sink.vtable[10], this, 0, ptr, 0, 0, 0x00270005, uintptr(unsafe.Pointer(created)))
	observations, skipped, err = sink.snapshot()
	if err != nil || skipped != 1 || len(observations) != 1 {
		t.Fatal("ignored copy accepted as completed")
	}
	runtime.KeepAlive(sink)
}

func TestWindowsTransferSinkCountsTwoDirectMoves(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil && err != syscall.Errno(1) {
		t.Fatal(err)
	}
	defer windows.CoUninitialize()
	root := resolvedTempDir(t)
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	request := TransferRequest{Paths: []string{filepath.Join(root, "first.txt"), filepath.Join(root, "second.txt")}, Directory: target, Move: true}
	sink := newTransferSink(request)
	defer sink.releaseSources()
	this := uintptr(unsafe.Pointer(sink))
	for _, source := range request.Paths {
		if err := os.WriteFile(source, []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
		item, err := shellItem(source)
		if err != nil {
			t.Fatal(err)
		}
		defer item.call(2)
		ptr := uintptr(unsafe.Pointer(item))
		if hr, _, _ := syscall.SyscallN(sink.vtable[7], this, 0, ptr, 0, 0); hr != 0 {
			t.Fatalf("pre-move refused: %x", hr)
		}
		destination := filepath.Join(target, filepath.Base(source))
		if err := os.Rename(source, destination); err != nil {
			t.Fatal(err)
		}
		created, err := shellItem(destination)
		if err != nil {
			t.Fatal(err)
		}
		defer created.call(2)
		// Same-volume native moves report this successful completion status.
		if hr, _, _ := syscall.SyscallN(sink.vtable[8], this, 0, ptr, 0, 0, 0x00270008, uintptr(unsafe.Pointer(created))); hr != 0 {
			t.Fatalf("post-move failed: %x", hr)
		}
	}
	observations, skipped, err := sink.snapshot()
	result := transferResults(request, request.Paths, observations, false, skipped)
	if err != nil || result.Skipped != 0 || len(result.Items) != 2 || len(result.Changes) != 2 {
		t.Fatalf("two completed moves reported incorrectly: %+v %v", result, err)
	}
	for _, item := range result.Items {
		if !item.Moved {
			t.Fatal("completed move not confirmed")
		}
	}
	runtime.KeepAlive(sink)
}

func TestWindowsTransferSinkInterfacesAndReferences(t *testing.T) {
	sink := newTransferSink(TransferRequest{})
	for _, iid := range []windows.GUID{iidUnknown, iidProgressSink, iidShellItem} {
		var out uintptr
		hr, _, _ := syscall.SyscallN(sink.vtable[0], uintptr(unsafe.Pointer(sink)), uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&out)))
		if iid == iidShellItem {
			if uint32(hr) != eNoInterface || out != 0 {
				t.Fatal("unsupported interface accepted")
			}
		} else {
			if hr != 0 || sink.refs.Load() != 2 {
				t.Fatal("supported interface/reference failed")
			}
			syscall.SyscallN(sink.vtable[2], out)
		}
	}
	if sink.refs.Load() != 1 {
		t.Fatal("unbalanced references")
	}
	runtime.KeepAlive(sink)
}

// Opt-in native execution shows real Windows progress/decision UI. Every file
// belongs to disposable directories. Optional roots exercise actual volumes and
// existing SUBST mappings without creating/changing any mapping.
func TestWindowsShellTransfers(t *testing.T) {
	if os.Getenv("COMMANDO_TEST_SHELL_TRANSFERS") != "1" {
		t.Skip("set COMMANDO_TEST_SHELL_TRANSFERS=1 on a Windows desktop")
	}
	source := t.TempDir()
	if root := os.Getenv("COMMANDO_TEST_SUBST_ROOT"); root != "" {
		var err error
		source, err = os.MkdirTemp(root, "commando-shell-test-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.RemoveAll(source); err != nil {
				t.Error(err)
			}
		})
	}
	target := t.TempDir()
	if root := os.Getenv("COMMANDO_TEST_TRANSFER_VOLUME"); root != "" {
		var err error
		target, err = os.MkdirTemp(root, "commando-shell-test-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.RemoveAll(target); err != nil {
				t.Error(err)
			}
		})
	}
	writeTransferFiles(t, source, map[string]string{"folder/child.txt": "nested", "file.txt": "data"})
	for _, move := range []bool{false, true} {
		directory := filepath.Join(target, "copy")
		if move {
			directory = filepath.Join(target, "move")
		}
		if err := os.Mkdir(directory, 0700); err != nil {
			t.Fatal(err)
		}
		result, err := Transfer(TransferRequest{Paths: []string{filepath.Join(source, "folder"), filepath.Join(source, "file.txt")}, Directory: directory, Move: move})
		if err != nil || result.Aborted || result.Skipped != 0 || len(result.Items) != 2 {
			t.Fatalf("native copy/move: %+v %v", result, err)
		}
		for _, item := range result.Items {
			if item.Moved != move {
				t.Fatal("incorrect move confirmation")
			}
		}
		for name, want := range map[string]string{"folder/child.txt": "nested", "file.txt": "data"} {
			data, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(name)))
			if err != nil || string(data) != want {
				t.Fatal("native contents:", err)
			}
		}
	}
}

func TestWindowsShellSUBSTMoveUsesRename(t *testing.T) {
	root := os.Getenv("COMMANDO_TEST_SUBST_ROOT")
	if os.Getenv("COMMANDO_TEST_SHELL_TRANSFERS") != "1" || root == "" {
		t.Skip("set shell-transfer opt-in and an existing SUBST root")
	}
	alias, err := os.MkdirTemp(root, "commando-subst-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(alias); err != nil {
			t.Error(err)
		}
	})
	backing, err := ResolveOperationPath(alias)
	if err != nil || sameTransferPath(alias, backing) {
		t.Fatalf("expected a SUBST alias: %s %v", backing, err)
	}
	writeTransferFiles(t, alias, map[string]string{"source/folder/child": "data"})
	target := filepath.Join(backing, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(filepath.Join(alias, "source", "folder", "child"))
	if err != nil || !os.SameFile(before, before) {
		t.Fatal("cannot freeze original identity:", err)
	}
	result, err := Transfer(TransferRequest{Paths: []string{filepath.Join(alias, "source", "folder")}, Directory: target, Move: true})
	if err != nil || result.Aborted || len(result.Items) != 1 || !result.Items[0].Moved {
		t.Fatalf("SUBST move: %+v %v", result, err)
	}
	after, err := os.Stat(filepath.Join(target, "folder", "child"))
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("same-volume SUBST move copied instead of renaming:", err)
	}
}
