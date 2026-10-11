package browser

import (
	"encoding/binary"
	"errors"
	"path/filepath"
	"unicode/utf16"
)

var ErrNoClipboardFiles = errors.New("clipboard contains no files")

type FileClipboard struct {
	Paths    []string
	Move     bool
	Sequence uint32
}

// Encode the native DROPFILES layout with a double-NUL UTF-16 file list.
func encodeFileDrop(paths []string) ([]byte, error) {
	if len(paths) == 0 {
		return nil, ErrNoClipboardFiles
	}
	words := make([]uint16, 0)
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			return nil, errors.New("clipboard path is not absolute")
		}
		for _, r := range path {
			if r == 0 {
				return nil, errors.New("invalid clipboard path")
			}
		}
		words = append(words, utf16.Encode([]rune(path))...)
		words = append(words, 0)
	}
	words = append(words, 0)
	data := make([]byte, 20+len(words)*2)
	binary.LittleEndian.PutUint32(data, 20)     // pFiles
	binary.LittleEndian.PutUint32(data[16:], 1) // fWide
	for i, word := range words {
		binary.LittleEndian.PutUint16(data[20+i*2:], word)
	}
	return data, nil
}

func remainingClipboardPaths(snapshot FileClipboard, moved []string) []string {
	var remaining []string
	for _, path := range snapshot.Paths {
		removed := false
		for _, completed := range moved {
			if path == completed {
				removed = true
				break
			}
		}
		if !removed {
			remaining = append(remaining, path)
		}
	}
	return remaining
}
