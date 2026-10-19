package browser

import (
	"archive/zip"
	"fmt"
	"io/fs"
	"math"
	"path/filepath"
	"slices"
	"strings"
)

// ArchiveAdapter keeps format details out of panel navigation and commands.
// Extract writes selected children into a private, empty staging directory.
type ArchiveAdapter interface {
	Capabilities() Capabilities
	ReadDirectory(Location) (Directory, error)
	Extract(Location, []string, string) error
	Delete(Location, []string, *archiveClipboardStamp) (RemovalResult, error)
	Rename(Location, string, string) error
	Add(Location, []string) (*ArchiveSources, error)
	Create(Location, string, bool, []byte) (Location, Entry, error)
	Duplicate(Location, []string, []string) error
	Move(Location, []string, Location, *archiveClipboardStamp) error
}

func ArchiveFormat(filename string) ArchiveAdapter {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".zip":
		return zipAdapter{}
	default:
		return nil
	}
}

func ReadArchive(location Location) (Directory, error) {
	adapter := ArchiveFormat(location.Path)
	if adapter == nil || !location.InArchive() || !fs.ValidPath(location.Folder) {
		return Directory{}, fs.ErrInvalid
	}
	return adapter.ReadDirectory(location)
}

var ErrInvalidArchive = zip.ErrFormat

type zipAdapter struct{}

func (zipAdapter) Capabilities() Capabilities {
	return Capabilities{Browse: true, Read: true, Add: true, Delete: true, Rename: true}
}

func (zipAdapter) ReadDirectory(location Location) (Directory, error) {
	r, err := zip.OpenReader(location.Path)
	if err != nil {
		if r != nil {
			r.Close()
		}
		return Directory{}, err
	}
	defer r.Close()
	prefix := ""
	if location.Folder != "." {
		prefix = location.Folder + "/"
	}
	found := location.Folder == "."
	children := make(map[string]Entry)
	for _, file := range r.File {
		name := strings.TrimSuffix(file.Name, "/")
		// Never interpret archive names as host paths or allow malformed names
		// to escape their archive. Implied folders do not need explicit records.
		if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, `\:`) {
			continue
		}
		if name == location.Folder && file.FileInfo().IsDir() {
			found = true
		}
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		rest := strings.TrimPrefix(name, prefix)
		if rest == "" {
			continue
		}
		found = true
		child, _, nested := strings.Cut(rest, "/")
		item := Entry{Name: child, Directory: nested || file.FileInfo().IsDir()}
		if !nested {
			item.Modified = file.Modified
			if !item.Directory && file.UncompressedSize64 <= math.MaxInt64 {
				item.Bytes, item.SizeKnown = int64(file.UncompressedSize64), true
			}
		}
		previous, exists := children[child]
		if !exists || item.Directory && !previous.Directory || !nested && item.Directory {
			children[child] = item
		}
	}
	if !found {
		return Directory{}, fmt.Errorf("%w: archive folder", fs.ErrNotExist)
	}
	entries := make([]Entry, 0, len(children)+1)
	for _, item := range children {
		entries = append(entries, item)
	}
	slices.SortFunc(entries, CompareEntries)
	entries = append([]Entry{{Name: "..", Directory: true, Parent: true}}, entries...)
	return Directory{Path: location.Path, ArchiveFolder: location.Folder, Entries: entries}, nil
}
