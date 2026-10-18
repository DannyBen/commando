package browser

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"github.com/dannyben/commando/internal/diagnostics"
	"golang.org/x/sys/windows"
)

func TestWindowsTransferTraceRecordsConflictCallbacksAndFinalOutcome(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil && err != syscall.Errno(1) {
		t.Fatal(err)
	}
	defer windows.CoUninitialize()
	root := resolvedTempDir(t)
	source, target := filepath.Join(root, "source.txt"), filepath.Join(root, "target")
	if err := os.WriteFile(source, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	item, err := shellItem(source)
	if err != nil {
		t.Fatal(err)
	}
	defer item.call(2)
	folder, err := shellItem(target)
	if err != nil {
		t.Fatal(err)
	}
	defer folder.call(2)
	tracePath, stopTrace, err := EnableTransferTrace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer stopTrace()
	sink := newTransferSink(TransferRequest{Paths: []string{source}, Directory: target})
	sink.trace = &transferBatchTrace{id: diagnostics.NextID()}
	defer sink.releaseSources()
	this, ptr, dest := uintptr(unsafe.Pointer(sink)), uintptr(unsafe.Pointer(item)), uintptr(unsafe.Pointer(folder))
	for _, status := range []uint32{0x0027000b, 0x00270005} {
		if hr, _, _ := syscall.SyscallN(sink.vtable[9], this, 2, ptr, dest, 0); hr != 0 {
			t.Fatalf("pre-copy refused: %x", hr)
		}
		if hr, _, _ := syscall.SyscallN(sink.vtable[10], this, 2, ptr, dest, 0, uintptr(status), 0); hr != 0 {
			t.Fatalf("post-copy refused: %x", hr)
		}
	}
	observations, summary, err := sink.snapshot(false)
	if err != nil || summary.Skipped != 1 || summary.Completed != 0 || len(observations) != 0 {
		t.Fatal("pending and final callbacks were not reduced to one skip")
	}
	traceData, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatal(err)
	}
	records := decodeTransferTrace(t, bytes.NewReader(traceData))
	if len(records) != 4 {
		t.Fatalf("callback events: %+v", records)
	}
	data := records[1].Data.(map[string]any)
	if data["source"].(map[string]any)["path"] != source || data["target"].(map[string]any)["path"] != target ||
		data["cached_source_path"] != source || data["created"] != nil || data["hresult"].(map[string]any)["hex"] != "0x0027000B" {
		t.Fatalf("missing conflict evidence: %+v", data)
	}
	runtime.KeepAlive(sink)
}

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
	observations, summary, err := sink.snapshot(false)
	if err != nil || summary.Skipped != 0 || summary.Completed != 1 || len(observations) != 1 || observations[0].source != source || observations[0].destination != destination {
		t.Fatalf("callback result: %+v %+v %v", observations, summary, err)
	}
	// A positive ignored status with a non-null target still isn't completion.
	syscall.SyscallN(sink.vtable[10], this, 0, ptr, 0, 0, 0x00270005, uintptr(unsafe.Pointer(created)))
	observations, summary, err = sink.snapshot(false)
	if err != nil || summary.Skipped != 0 || summary.Completed != 1 || len(observations) != 1 {
		t.Fatal("a later ignored callback counted a completed source twice")
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
	observations, summary, err := sink.snapshot(false)
	result := transferResults(request, request.Paths, observations, false, summary)
	if err != nil || result.Summary.Skipped != 0 || result.Summary.Completed != 2 || len(result.Items) != 2 || len(result.Changes) != 2 {
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
		if err != nil || result.Aborted || result.Summary.Skipped != 0 || result.Summary.Completed != 2 || len(result.Items) != 2 {
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
