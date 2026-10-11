package browser

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestReadDirectory(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"z-folder", "A-folder"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"z.txt", "A.txt", "empty.txt"} {
		content := []byte("abc")
		if name == "empty.txt" {
			content = nil
		}
		if err := os.WriteFile(filepath.Join(root, name), content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	modified := time.Date(2024, time.March, 12, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(root, "A.txt"), modified, modified); err != nil {
		t.Fatal(err)
	}
	directory, err := ReadDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, item := range directory.Entries {
		names = append(names, item.Name)
	}
	want := []string{"..", "A-folder", "z-folder", "A.txt", "empty.txt", "z.txt"}
	if directory.Path != root || !reflect.DeepEqual(names, want) {
		t.Fatalf("directory=%+v, want path %s and names %v", directory, root, want)
	}
	if !directory.Entries[0].Parent || !directory.Entries[1].Directory {
		t.Fatal("parent/folder metadata missing")
	}
	if item := directory.Entries[3]; !item.SizeKnown || item.Bytes != 3 {
		t.Fatalf("file metadata=%+v", item)
	}
	if !directory.Entries[3].Modified.Equal(modified) || !directory.Entries[0].Modified.IsZero() {
		t.Fatal("modification time was lost or fabricated for the parent row")
	}
	if item := directory.Entries[4]; !item.SizeKnown || item.Bytes != 0 {
		t.Fatalf("empty file metadata=%+v", item)
	}
	if _, err := ReadDirectory(filepath.Join(root, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing folder error=%v", err)
	}
	if _, err := ReadDirectory(filepath.Join(root, "A.txt")); err == nil {
		t.Fatal("opening an ordinary file as a directory succeeded")
	}
}

func TestDirectoryLinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "folder")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "linked-folder")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, "broken-link")); err != nil {
		t.Fatal(err)
	}
	directory, err := ReadDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	if item := directory.Entries[1]; item.Name != "linked-folder" || !item.Directory {
		t.Fatalf("folder link is not navigable: %+v", item)
	}
	if item := directory.Entries[2]; item.Name != "broken-link" || item.SizeKnown {
		t.Fatalf("broken link disappeared or has fabricated metadata: %+v", item)
	}
}
