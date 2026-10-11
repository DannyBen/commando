package browser

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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
