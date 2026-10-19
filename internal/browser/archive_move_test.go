package browser

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMoveWithinArchiveKeepsCompressedDataAndRejectsConflicts(t *testing.T) {
	filename := archiveFixture(t)
	location := Location{Path: filename, Folder: "."}
	reference, err := CaptureArchiveClipboard(location, []string{"folder", "a2.txt"})
	if err != nil {
		t.Fatal(err)
	}
	comment, before, headers := zipMembers(t, filename)
	if err := MoveArchive(reference, Location{Path: filename, Folder: "empty"}); err != nil {
		t.Fatal(err)
	}
	afterComment, after, afterHeaders := zipMembers(t, filename)
	if afterComment != comment || len(before) != len(after) {
		t.Fatal("move changed archive metadata/count")
	}
	for name, data := range before {
		target := name
		if name == "folder/sub/file.txt" || name == "a2.txt" {
			target = "empty/" + name
		}
		if !bytes.Equal(data, after[target]) {
			t.Fatal("move changed compressed payload", name)
		}
		expected := headers[name]
		if target != name {
			expected.Name = target
			expected.Flags |= 0x800
			expected.NonUTF8 = false
		}
		if !reflect.DeepEqual(expected, afterHeaders[target]) {
			t.Fatal("move changed metadata", name)
		}
	}
	reference, err = CaptureArchiveClipboard(Location{Path: filename, Folder: "empty"}, []string{"folder"})
	if err != nil {
		t.Fatal(err)
	}
	unchanged, _ := os.ReadFile(filename)
	if err := MoveArchive(reference, Location{Path: filename, Folder: "empty/folder/sub"}); !errors.Is(err, fs.ErrInvalid) {
		t.Fatal("move into itself accepted", err)
	}
	next, _ := os.ReadFile(filename)
	if !bytes.Equal(unchanged, next) {
		t.Fatal("invalid move changed ZIP")
	}
	if err := MoveArchive(reference, reference.Location); err != nil {
		t.Fatal(err)
	}
	next, _ = os.ReadFile(filename)
	if !bytes.Equal(unchanged, next) {
		t.Fatal("same-folder move rewrote ZIP")
	}
}

func TestArchiveReferenceRemovalAndRemainingCut(t *testing.T) {
	filename := archiveFixture(t)
	reference, err := CaptureArchiveClipboard(Location{Path: filename, Folder: "."}, []string{"a2.txt", "folder"})
	if err != nil {
		t.Fatal(err)
	}
	reference.Move = true
	if err := reference.Remove([]string{"empty"}); !errors.Is(err, fs.ErrInvalid) {
		t.Fatal("removed outside captured selection", err)
	}
	if err := reference.Remove([]string{"a2.txt"}); err != nil {
		t.Fatal(err)
	}
	remaining, err := reference.Remaining([]string{"a2.txt"})
	if err != nil || remaining == nil || !remaining.Move || !reflect.DeepEqual(remaining.Names, []string{"folder"}) {
		t.Fatal("remaining cut not refreshed", remaining, err)
	}
	stage, err := remaining.Stage()
	if err != nil {
		t.Fatal("partial cut became stale", err)
	}
	stage.Close()
	if err := reference.Remove([]string{"folder"}); !errors.Is(err, ErrArchiveClipboardChanged) {
		t.Fatal("old reference could remove changed archive", err)
	}
}

func TestArchiveSourceReceiptRefusesChangedTreesAndItems(t *testing.T) {
	for _, kind := range []string{"changed", "new child", "replaced"} {
		t.Run(kind, func(t *testing.T) {
			filename := archiveFixture(t)
			root := t.TempDir()
			source := filepath.Join(root, "source")
			if err := os.Mkdir(source, 0700); err != nil {
				t.Fatal(err)
			}
			child := filepath.Join(source, "file.txt")
			if err := os.WriteFile(child, []byte("data"), 0600); err != nil {
				t.Fatal(err)
			}
			receipt, err := AddArchiveSources(Location{Path: filename, Folder: "."}, []string{source})
			if err != nil {
				t.Fatal(err)
			}
			defer receipt.Close()
			if err := receipt.prepareRemoval(); err != nil {
				t.Fatal("unchanged receipt refused", err)
			}
			if err := receipt.validateItem(child); err != nil {
				t.Fatal(err)
			}
			if kind == "new child" {
				err = os.WriteFile(filepath.Join(source, "new.txt"), []byte("new"), 0600)
			} else if kind == "replaced" {
				err = os.Rename(child, child+".old")
				if err == nil {
					err = os.WriteFile(child, []byte("data"), 0600)
				}
			} else {
				err = os.WriteFile(child, []byte("changed"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := receipt.validate(); !errors.Is(err, ErrArchiveSourceChanged) {
				t.Fatal("changed tree accepted", err)
			}
			candidate := child
			if kind == "new child" {
				candidate = filepath.Join(source, "new.txt")
			}
			if err := receipt.validateItem(candidate); !errors.Is(err, ErrArchiveSourceChanged) {
				t.Fatal("changed item accepted", err)
			}
			if _, err := os.Stat(child); err != nil {
				t.Fatal("validation removed source", err)
			}
		})
	}
}

func TestArchiveSourceRemovalRequiresCommittedDestination(t *testing.T) {
	filename := archiveFixture(t)
	source := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(source, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	receipt, err := AddArchiveSources(Location{Path: filename, Folder: "."}, []string{source})
	if err != nil {
		t.Fatal(err)
	}
	defer receipt.Close()
	if _, _, err := CreateArchive(Location{Path: filename, Folder: "."}, "changed.txt", false, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveArchiveSources(receipt, 0); !errors.Is(err, ErrArchiveChanged) {
		t.Fatal("changed destination did not veto removal", err)
	}
	if data, err := os.ReadFile(source); err != nil || string(data) != "data" {
		t.Fatal("source lost", err)
	}
}
