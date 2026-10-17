package browser

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateFolder(t *testing.T) {
	root := t.TempDir()
	item, err := CreateFolder(root, "Å new folder")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, item.Name))
	if err != nil || !info.IsDir() || !item.Directory || item.Modified.IsZero() {
		t.Fatalf("created folder: %+v %v", item, err)
	}
	if _, err := CreateFolder(root, item.Name); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("existing directory accepted: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "existing"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateFolder(root, "existing"); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("existing file accepted: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "existing")); err != nil || string(data) != "keep" {
		t.Fatal("creation changed an existing file")
	}
}

func TestCreateEmptyFileNeverReplacesEntries(t *testing.T) {
	root := t.TempDir()
	item, err := CreateFile(root, "new.txt")
	if err != nil || item.Directory || !item.SizeKnown || item.Bytes != 0 || item.Modified.IsZero() {
		t.Fatalf("created file metadata: %+v %v", item, err)
	}
	path := filepath.Join(root, "new.txt")
	if data, err := os.ReadFile(path); err != nil || len(data) != 0 {
		t.Fatal("new file is not empty")
	}
	if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateFile(root, "new.txt"); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("existing file accepted: %v", err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "keep" {
		t.Fatal("creation replaced or truncated an existing file")
	}
	if err := os.Mkdir(filepath.Join(root, "folder"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateFile(root, "folder"); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("existing folder accepted: %v", err)
	}
	if _, err := CreateFile(root, "../outside"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("path accepted: %v", err)
	}
}

func TestCreateFolderRejectsPathsAndInvalidNames(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"", ".", "..", "../outside", "nested/folder", `nested\folder`, "CON", "name.", "name "} {
		if _, err := CreateFolder(root, name); !errors.Is(err, ErrInvalidName) {
			t.Fatalf("%q accepted: %v", name, err)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("invalid names created entries")
	}
	if _, err := CreateFolder(filepath.Join(root, "missing"), "child"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing parent: %v", err)
	}
}

func TestCreatePathMakesParentsAndNeverReplacesTarget(t *testing.T) {
	for _, folder := range []bool{false, true} {
		for _, name := range []string{"new/sub/item.txt", `new\sub\item.txt`} {
			t.Run(fmt.Sprintf("folder=%v/%s", folder, name), func(t *testing.T) {
				root := t.TempDir()
				parent, item, err := CreatePath(root, name, folder, []byte("contents"))
				if err != nil || parent != filepath.Join(root, "new", "sub") || item.Name != "item.txt" || item.Directory != folder {
					t.Fatalf("created path: parent=%s item=%+v error=%v", parent, item, err)
				}
				if _, _, err := CreatePath(root, name, folder, nil); !errors.Is(err, fs.ErrExist) {
					t.Fatal("existing target was accepted", err)
				}
				if !folder {
					data, err := os.ReadFile(filepath.Join(parent, item.Name))
					if err != nil || string(data) != "contents" {
						t.Fatal("collision changed existing file", err)
					}
				}
				if _, _, err := CreatePath(root, "new/sub/another", folder, nil); err != nil {
					t.Fatal("existing parents could not be reused", err)
				}
			})
		}
	}
}

func TestCreatePathValidatesWholePathBeforeMutation(t *testing.T) {
	for _, name := range []string{"", "/absolute", `C:\absolute`, "a/../escape", "a/./file", "a//file", "a/CON/file", "a/invalid?", "a/trailing.", "a/"} {
		root := t.TempDir()
		if _, _, err := CreatePath(root, name, false, nil); !errors.Is(err, ErrInvalidName) {
			t.Fatalf("invalid path %q accepted: %v", name, err)
		}
		files, err := os.ReadDir(root)
		if err != nil || len(files) != 0 {
			t.Fatalf("invalid path %q created parents", name)
		}
	}
}

func TestCreatePathRetainsPartialParentsAndRetries(t *testing.T) {
	root := t.TempDir()
	// A component exceeding the filesystem limit fails after MkdirAll has
	// created the preceding parents. No permission assumptions or races needed.
	name := "new/sub/" + strings.Repeat("x", 300)
	if _, _, err := CreatePath(root, name, true, nil); err == nil {
		t.Fatal("overlong final component unexpectedly succeeded")
	}
	if info, err := os.Stat(filepath.Join(root, "new", "sub")); err != nil || !info.IsDir() {
		t.Fatal("partial parents were removed", err)
	}
	if _, _, err := CreatePath(root, "new/sub/retry", true, nil); err != nil {
		t.Fatal("retry could not reuse partial parents", err)
	}
}
