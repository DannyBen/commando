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
