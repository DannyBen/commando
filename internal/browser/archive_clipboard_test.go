package browser

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestArchiveClipboardDefersExtractionAndRoundTrips(t *testing.T) {
	filename := archiveFixture(t)
	scratch := t.TempDir()
	t.Setenv("TMPDIR", scratch)
	t.Setenv("TMP", scratch)
	t.Setenv("TEMP", scratch)
	reference, err := CaptureArchiveClipboard(Location{Path: filename, Folder: "."}, []string{"folder", "a2.txt"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := encodeArchiveClipboard(reference)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeArchiveClipboard(data)
	if err != nil || !reflect.DeepEqual(decoded, reference) {
		t.Fatal("clipboard reference round trip", decoded, err)
	}
	entries, err := os.ReadDir(scratch)
	if err != nil || len(entries) != 0 {
		t.Fatal("Copy extracted files", entries, err)
	}
	for range 2 {
		stage, err := decoded.Stage()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.ReadFile(filepath.Join(stage.Directory, "folder", "sub", "file.txt")); err != nil {
			t.Fatal(err)
		}
		if err := stage.Close(); err != nil {
			t.Fatal(err)
		}
	}
	entries, err = os.ReadDir(filepath.Join(scratch, "Commando", "session"))
	if err != nil || len(entries) != 0 {
		t.Fatal("paste leaked files", entries, err)
	}
}

func TestArchiveClipboardRefusesChangedReplacedAndMissingSource(t *testing.T) {
	for _, kind := range []string{"changed", "replaced", "missing"} {
		t.Run(kind, func(t *testing.T) {
			filename := archiveFixture(t)
			reference, err := CaptureArchiveClipboard(Location{Path: filename, Folder: "."}, []string{"a2.txt"})
			if err != nil {
				t.Fatal(err)
			}
			info, _ := os.Stat(filename)
			original, _ := os.ReadFile(filename)
			if kind == "missing" {
				err = os.Remove(filename)
			} else if kind == "replaced" {
				// Retain the old inode so a fresh file cannot reuse its identity.
				err = os.Rename(filename, filename+".old")
				if err == nil {
					err = os.WriteFile(filename, original, 0600)
				}
				if err == nil {
					err = os.Chtimes(filename, info.ModTime(), info.ModTime())
				}
			} else {
				err = os.WriteFile(filename, append(original, 'x'), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			stage, err := reference.Stage()
			if stage != nil {
				stage.Close()
				t.Fatal("stale reference extracted files")
			}
			if kind == "missing" && !errors.Is(err, fs.ErrNotExist) || kind != "missing" && !errors.Is(err, ErrArchiveClipboardChanged) {
				t.Fatal("wrong stale-source error", err)
			}
		})
	}
}

func TestArchiveClipboardRejectsInvalidPayloadsAndSelections(t *testing.T) {
	filename := archiveFixture(t)
	for _, names := range [][]string{nil, {"missing"}, {"../escape"}, {"a2.txt", "a2.txt"}} {
		if _, err := CaptureArchiveClipboard(Location{Path: filename, Folder: "."}, names); err == nil {
			t.Fatal("invalid capture", names)
		}
	}
	for _, data := range [][]byte{nil, []byte("null"), []byte("{}"), []byte("{broken"), make([]byte, maxArchiveClipboardBytes+1)} {
		if _, err := decodeArchiveClipboard(data); err == nil {
			t.Fatal("invalid payload accepted")
		}
	}
}
