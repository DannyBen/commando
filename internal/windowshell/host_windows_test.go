package windowshell

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

func TestNativeCommandABI(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("probe targets amd64")
	}
	var info invokeInfo
	if unsafe.Sizeof(info) != 104 || unsafe.Offsetof(info.VerbW) != 64 || unsafe.Offsetof(info.Point) != 96 {
		t.Fatal("CMINVOKECOMMANDINFOEX does not match the Windows amd64 ABI")
	}
}

func TestHRESULT(t *testing.T) {
	for _, value := range []uintptr{0, 1, 0x40100, 0x40101} {
		if err := hresult(value); err != nil {
			t.Fatalf("success status rejected: %v", err)
		}
	}
	if !errors.Is(hresult(0x80070005), syscall.Errno(5)) {
		t.Fatal("access denied HRESULT lost its OS reason")
	}
	if hresult(0x80004005) == nil {
		t.Fatal("failed HRESULT accepted")
	}
}

// This checks real shell objects on Windows, but not a Gio drag loop or an
// interactive menu. Those require the separate executable's desktop review.
func TestShellObjectsForFilesAndBatch(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := oleInitialize.Call(0)
	if err := hresult(hr); err != nil {
		t.Fatal(err)
	}
	defer oleUninitialize.Call()
	root := t.TempDir()
	paths := []string{filepath.Join(root, "two words.txt"), filepath.Join(root, "café-日本語.txt")}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("sample"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, selection := range [][]string{paths[:1], paths} {
		for _, iid := range []*windows.GUID{&iidData, &iidMenu} {
			object, err := shellObject(0, selection, iid)
			if err != nil {
				t.Fatal(err)
			}
			if object == nil {
				t.Fatal("shell returned a nil object")
			}
			object.call(2)
		}
	}
}
