package browser

import (
	"encoding/binary"
	"path/filepath"
	"reflect"
	"testing"
	"unicode/utf16"
)

func TestFileDropUnicodeAndTerminators(t *testing.T) {
	paths := []string{filepath.Join(t.TempDir(), "Å résumé.txt"), filepath.Join(t.TempDir(), "📁 folder")}
	data, err := encodeFileDrop(paths)
	if err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint32(data) != 20 || binary.LittleEndian.Uint32(data[16:]) != 1 {
		t.Fatal("invalid DROPFILES header")
	}
	var words []uint16
	for i := 20; i < len(data); i += 2 {
		words = append(words, binary.LittleEndian.Uint16(data[i:]))
	}
	var decoded []string
	start := 0
	for i, word := range words {
		if word == 0 {
			if i == start {
				break
			}
			decoded = append(decoded, string(utf16.Decode(words[start:i])))
			start = i + 1
		}
	}
	if !reflect.DeepEqual(decoded, paths) || words[len(words)-1] != 0 || words[len(words)-2] != 0 {
		t.Fatal("Unicode paths or double terminator lost")
	}
}

func TestFileDropRejectsEmptyRelativeAndNulPaths(t *testing.T) {
	for _, paths := range [][]string{nil, {"relative"}, {filepath.Join(t.TempDir(), "bad\x00name")}} {
		if _, err := encodeFileDrop(paths); err == nil {
			t.Fatalf("invalid drop accepted: %q", paths)
		}
	}
}

func TestClipboardPartialMoveRetainsUnprocessedPaths(t *testing.T) {
	snapshot := FileClipboard{Paths: []string{"a", "b", "c"}, Move: true}
	if got := remainingClipboardPaths(snapshot, []string{"a", "c"}); !reflect.DeepEqual(got, []string{"b"}) {
		t.Fatalf("partial clipboard: %v", got)
	}
}
