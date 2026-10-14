package browser

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func createTestShortcut(t *testing.T, path, target string) {
	t.Helper()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil && err != syscall.Errno(1) {
		t.Fatal(err)
	}
	defer windows.CoUninitialize()
	var link *shellObject
	hr, _, _ := coCreateInstance.Call(uintptr(unsafe.Pointer(&clsidShellLink)), 0, 1, uintptr(unsafe.Pointer(&iidShellLink)), uintptr(unsafe.Pointer(&link)))
	if err := shellHRESULT(uint32(hr)); err != nil {
		t.Fatal(err)
	}
	defer link.call(2)
	targetName, err := windows.UTF16PtrFromString(target)
	if err != nil {
		t.Fatal(err)
	}
	hr32 := link.call(20, uintptr(unsafe.Pointer(targetName)))
	runtime.KeepAlive(targetName)
	if err := shellHRESULT(hr32); err != nil {
		t.Fatal(err)
	}
	var persisted *shellObject
	if err := shellHRESULT(link.call(0, uintptr(unsafe.Pointer(&iidPersistFile)), uintptr(unsafe.Pointer(&persisted)))); err != nil {
		t.Fatal(err)
	}
	defer persisted.call(2)
	fileName, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	hr32 = persisted.call(6, uintptr(unsafe.Pointer(fileName)), 1)
	runtime.KeepAlive(fileName)
	if err := shellHRESULT(hr32); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsFolderShortcutTargetUsesShellWithoutChangingLink(t *testing.T) {
	root := resolvedTempDir(t)
	for _, name := range []string{"folder with spaces", "תיקיה"} {
		target := filepath.Join(root, name)
		if err := os.Mkdir(target, 0700); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(root, name+".lnk")
		createTestShortcut(t, link, target)
		before, err := os.ReadFile(link)
		if err != nil {
			t.Fatal(err)
		}
		folder, err := FolderShortcutTarget(link)
		if err != nil || !strings.EqualFold(filepath.Clean(folder), filepath.Clean(target)) {
			t.Fatalf("folder target = %q, %v; want %q", folder, err, target)
		}
		after, err := os.ReadFile(link)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("resolving shortcut changed its contents", err)
		}
	}
}

func TestWindowsFileAndInvalidShortcutsHaveNoFolderTarget(t *testing.T) {
	root := resolvedTempDir(t)
	target := filepath.Join(root, "program.exe")
	if err := os.WriteFile(target, []byte("not executed"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "program.lnk")
	createTestShortcut(t, link, target)
	if folder, err := FolderShortcutTarget(link); err != nil || folder != "" {
		t.Fatalf("file target was treated as folder: %q, %v", folder, err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if folder, _ := FolderShortcutTarget(link); folder != "" {
		t.Fatal("broken shortcut returned a folder")
	}
	if err := os.WriteFile(link, []byte("invalid shortcut"), 0600); err != nil {
		t.Fatal(err)
	}
	if folder, err := FolderShortcutTarget(link); err == nil || folder != "" {
		t.Fatal("malformed shortcut accepted")
	}
}
