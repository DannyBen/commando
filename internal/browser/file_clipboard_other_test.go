//go:build !windows

package browser

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestClipboardMoveCompletionAndReplacement(t *testing.T) {
	paths := []string{filepath.Join(t.TempDir(), "a"), filepath.Join(t.TempDir(), "b")}
	snapshot, err := WriteFileClipboard(paths, true)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := CompleteFileClipboard(snapshot, paths[:1])
	if err != nil || len(updated.Paths) != 1 || updated.Paths[0] != paths[1] || updated.Sequence == snapshot.Sequence {
		t.Fatalf("partial cut completion: %+v %v", updated, err)
	}
	newer, err := WriteFileClipboard(paths[:1], false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = CompleteFileClipboard(updated, paths[1:])
	current, _ := ReadFileClipboard()
	if err != nil || current.Sequence != newer.Sequence || current.Move {
		t.Fatal("move completion overwrote newer clipboard")
	}
	cut, _ := WriteFileClipboard(paths, true)
	_, err = CompleteFileClipboard(cut, paths)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFileClipboard(); !errors.Is(err, ErrNoClipboardFiles) {
		t.Fatal("completed cut remained reusable")
	}
}

func TestDeferredArchiveClipboardReplacement(t *testing.T) {
	reference, err := CaptureArchiveClipboard(Location{Path: archiveFixture(t), Folder: "."}, []string{"a2.txt"})
	if err != nil {
		t.Fatal(err)
	}
	written, err := WriteArchiveClipboard(reference)
	if err != nil {
		t.Fatal(err)
	}
	read, err := ReadFileClipboard()
	if err != nil || read.Archive == nil || len(read.Paths) != 0 || read.Move || read.Sequence != written.Sequence {
		t.Fatal("archive clipboard round trip", read, err)
	}
	read.Archive.Names[0] = "changed"
	fresh, _ := ReadFileClipboard()
	if fresh.Archive.Names[0] != "a2.txt" {
		t.Fatal("clipboard aliases caller memory")
	}
	if _, err := WriteFileClipboard([]string{filepath.Join(t.TempDir(), "normal.txt")}, false); err != nil {
		t.Fatal(err)
	}
	replaced, err := ReadFileClipboard()
	if err != nil || replaced.Archive != nil || len(replaced.Paths) != 1 {
		t.Fatal("normal clipboard did not replace references", replaced, err)
	}
}
