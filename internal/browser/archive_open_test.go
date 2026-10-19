package browser

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenArchiveRetainsReadOnlyCopyAndPreservesOriginal(t *testing.T) {
	filename := archiveFixture(t)
	before, _ := os.ReadFile(filename)
	var opened string
	err := OpenArchive(Location{Path: filename, Folder: "folder/sub"}, "file.txt", func(name string) error {
		opened = name
		info, err := os.Stat(name)
		if err != nil {
			return err
		}
		if filepath.Base(name) != "file.txt" || info.Mode().Perm()&0222 != 0 {
			t.Fatalf("not a read-only extracted file: %s %v", name, info.Mode())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { os.Chmod(opened, 0600); os.RemoveAll(filepath.Dir(opened)) }()
	if _, err := os.ReadFile(opened); err != nil {
		t.Fatal("copy removed before external application finished", err)
	}
	after, _ := os.ReadFile(filename)
	if !bytes.Equal(before, after) {
		t.Fatal("Open changed archive")
	}
}

func TestOpenArchiveFailureCleansTemporaryCopy(t *testing.T) {
	filename := archiveFixture(t)
	scratch := t.TempDir()
	t.Setenv("TMPDIR", scratch)
	t.Setenv("TMP", scratch)
	t.Setenv("TEMP", scratch)
	failure := errors.New("launch failed")
	err := OpenArchive(Location{Path: filename, Folder: "."}, "a2.txt", func(name string) error { return failure })
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	for _, name := range []string{"empty", "missing.txt", "../escape"} {
		if err := OpenArchive(Location{Path: filename, Folder: "."}, name, func(string) error { t.Fatal("invalid file was opened"); return nil }); err == nil {
			t.Fatal("invalid selection accepted", name)
		}
	}
	if err := OpenArchive(Location{Path: filename, Folder: "."}, "a2.txt", nil); err == nil {
		t.Fatal("nil opener accepted")
	}
	entries, err := os.ReadDir(scratch)
	if err != nil || len(entries) != 0 {
		t.Fatal("failed opening leaked temporary files", entries, err)
	}
}
