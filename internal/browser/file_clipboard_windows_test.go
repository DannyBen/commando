package browser

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Opt in on a Windows desktop. This test replaces the system clipboard.
func TestWindowsFileClipboardRoundTrip(t *testing.T) {
	if os.Getenv("COMMANDO_TEST_CLIPBOARD") != "1" {
		t.Skip("requires COMMANDO_TEST_CLIPBOARD=1 on a Windows desktop; replaces clipboard")
	}
	paths := []string{filepath.Join(t.TempDir(), "Å file.txt"), filepath.Join(t.TempDir(), "📁 folder")}
	for _, move := range []bool{false, true} {
		snapshot, err := WriteFileClipboard(paths, move)
		if err != nil {
			t.Fatal(err)
		}
		read, err := ReadFileClipboard()
		if err != nil || read.Move != move || !reflect.DeepEqual(read.Paths, paths) || read.Sequence != snapshot.Sequence {
			t.Fatalf("native round trip: %+v %v", read, err)
		}
	}
	cut, _ := ReadFileClipboard()
	partial, err := CompleteFileClipboard(cut, paths[:1])
	if err != nil || !reflect.DeepEqual(partial.Paths, paths[1:]) {
		t.Fatalf("native partial completion: %+v %v", partial, err)
	}
	read, err := ReadFileClipboard()
	if err != nil || !reflect.DeepEqual(read, partial) {
		t.Fatalf("native partial payload: %+v %v", read, err)
	}
	newer, err := WriteFileClipboard(paths, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompleteFileClipboard(partial, paths[1:]); err != nil {
		t.Fatal(err)
	}
	read, err = ReadFileClipboard()
	if err != nil || !reflect.DeepEqual(read, newer) {
		t.Fatal("native completion replaced newer clipboard")
	}
	cut, err = WriteFileClipboard(paths, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompleteFileClipboard(cut, paths); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFileClipboard(); !errors.Is(err, ErrNoClipboardFiles) {
		t.Fatal("native completed cut retained file payload")
	}
}
