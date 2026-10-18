package browser

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestShortcutBatchPreservesSourcesAndExistingDestinations(t *testing.T) {
	root := resolvedTempDir(t)
	file, folder := filepath.Join(root, "notes.txt"), filepath.Join(root, "folder")
	if err := os.WriteFile(file, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(root, "notes.txt - Shortcut.lnk")
	if err := os.WriteFile(existing, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "folder - Shortcut.lnk"), 0700); err != nil {
		t.Fatal(err)
	}
	result, err := createShortcuts([]string{file, folder, file}, root, func(target string) ([]byte, error) {
		return []byte(target), nil
	})
	if err != nil || result.Summary.Completed != 3 || !result.Attempted {
		t.Fatal("shortcut batch failed", result, err)
	}
	for i, name := range []string{"notes.txt - Shortcut (2).lnk", "folder - Shortcut (2).lnk", "notes.txt - Shortcut (3).lnk"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(data) != result.Items[i].Source || result.Items[i].Moved || result.Items[i].Entry.Directory {
			t.Fatal("shortcut did not preserve its target", name, string(data), err)
		}
	}
	for path, want := range map[string]string{file: "original", existing: "keep"} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Fatal("source or existing destination changed", path, err)
		}
	}
}

func TestShortcutBatchRetainsCompletedLinksOnFailure(t *testing.T) {
	root := t.TempDir()
	var paths []string
	for _, name := range []string{"first", "second", "third"} {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	result, err := createShortcuts(paths, root, func(target string) ([]byte, error) {
		if filepath.Base(target) == "second" {
			return nil, fs.ErrPermission
		}
		return []byte("link"), nil
	})
	if !errors.Is(err, fs.ErrPermission) || result.Summary.Completed != 1 || result.Summary.Failed != 1 || result.Summary.Unfinished != 1 {
		t.Fatal("partial result was lost", result, err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "first - Shortcut.lnk")); err != nil || string(data) != "link" {
		t.Fatal("completed shortcut was removed", err)
	}
	for _, name := range []string{"second", "third"} {
		if _, err := os.Stat(filepath.Join(root, name+" - Shortcut.lnk")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatal("failed or unattempted shortcut was created", name, err)
		}
	}
}
