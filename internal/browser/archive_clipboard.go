package browser

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

var ErrArchiveClipboardChanged = errors.New("copied archive changed")

const maxArchiveClipboardBytes = 4 << 20

// ArchiveClipboard is a deferred selection, never a set of extracted files.
// Its stamp rejects a replaced or changed source instead of silently copying
// different contents. It is a validity check, not a content snapshot.
type ArchiveClipboard struct {
	Location Location
	Names    []string
	Stamp    archiveClipboardStamp
}

type archiveClipboardStamp struct {
	Identity string
	Size     int64
	Modified int64
}

func CaptureArchiveClipboard(location Location, names []string) (*ArchiveClipboard, error) {
	resolved, err := ResolveOperationPath(location.Path)
	if err != nil {
		return nil, err
	}
	location.Path, err = filepath.EvalSymlinks(resolved)
	if err != nil {
		return nil, err
	}
	reference := &ArchiveClipboard{Location: location, Names: slices.Clone(names)}
	if err := reference.valid(); err != nil {
		return nil, err
	}
	reference.Stamp, err = readArchiveClipboardStamp(location.Path)
	if err != nil {
		return nil, err
	}
	directory, err := ReadArchive(location)
	if err != nil {
		return nil, err
	}
	found := map[string]bool{}
	for _, entry := range directory.Entries {
		if !entry.Parent {
			found[entry.Name] = true
		}
	}
	for _, name := range names {
		if !found[name] {
			return nil, fs.ErrNotExist
		}
	}
	if err := reference.check(); err != nil {
		return nil, err
	}
	return reference, nil
}

func (reference *ArchiveClipboard) valid() error {
	if reference == nil || !filepath.IsAbs(reference.Location.Path) || !reference.Location.InArchive() || !fs.ValidPath(reference.Location.Folder) || !reference.Location.Capabilities().Read || len(reference.Names) == 0 || len(reference.Names) > 100000 {
		return fs.ErrInvalid
	}
	seen := map[string]bool{}
	for _, name := range reference.Names {
		if err := ValidateName(name); err != nil {
			return err
		}
		if seen[name] {
			return fs.ErrInvalid
		}
		seen[name] = true
	}
	return nil
}

func readArchiveClipboardStamp(filename string) (archiveClipboardStamp, error) {
	source, err := os.Open(filename)
	if err != nil {
		return archiveClipboardStamp{}, err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return archiveClipboardStamp{}, err
	}
	if !info.Mode().IsRegular() {
		return archiveClipboardStamp{}, fs.ErrInvalid
	}
	identity, err := archiveClipboardIdentity(source, info)
	return archiveClipboardStamp{Identity: identity, Size: info.Size(), Modified: info.ModTime().UnixNano()}, err
}

func (reference *ArchiveClipboard) check() error {
	if err := reference.valid(); err != nil {
		return err
	}
	current, err := readArchiveClipboardStamp(reference.Location.Path)
	if err != nil {
		return err
	}
	if reference.Stamp != current {
		return ErrArchiveClipboardChanged
	}
	return nil
}

func (reference *ArchiveClipboard) Stage() (*ArchiveStage, error) {
	if err := reference.check(); err != nil {
		return nil, err
	}
	stage, err := StageArchive(reference.Location, reference.Names)
	if err != nil {
		return nil, err
	}
	if err := reference.check(); err != nil {
		stage.Close()
		return nil, err
	}
	return stage, nil
}

func encodeArchiveClipboard(reference *ArchiveClipboard) ([]byte, error) {
	if err := reference.valid(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(reference)
	if err == nil && len(data) > maxArchiveClipboardBytes {
		return nil, fs.ErrInvalid
	}
	return data, err
}

func decodeArchiveClipboard(data []byte) (*ArchiveClipboard, error) {
	if len(data) == 0 || len(data) > maxArchiveClipboardBytes {
		return nil, fs.ErrInvalid
	}
	var reference ArchiveClipboard
	if err := json.Unmarshal(data, &reference); err != nil {
		return nil, err
	}
	if err := reference.valid(); err != nil {
		return nil, err
	}
	return &reference, nil
}
