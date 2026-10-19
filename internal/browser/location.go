package browser

import (
	"os"
	"path"
	"path/filepath"
)

// Path is always a real filesystem path. Folder is empty for filesystem
// locations, or an archive-relative slash path ("." at the archive root).
type Location struct {
	Path, Folder string
}

// Capabilities describe implemented operations, independently for each format.
// NativePaths allows existing filesystem/shell handlers to receive real paths.
type Capabilities struct {
	Browse, Read, Add, Delete, Rename, NativePaths bool
}

func (l Location) InArchive() bool { return l.Folder != "" }

func (l Location) Capabilities() Capabilities {
	if l.InArchive() {
		if adapter := ArchiveFormat(l.Path); adapter != nil {
			return adapter.Capabilities()
		}
		return Capabilities{}
	}
	return Capabilities{Browse: true, Read: true, Add: true, Delete: true, Rename: true, NativePaths: true}
}

func (l Location) String() string {
	if l.InArchive() {
		return filepath.Join(l.Path, filepath.FromSlash(l.Folder))
	}
	return l.Path
}

func (l Location) Child(name string) Location {
	if l.InArchive() {
		l.Folder = path.Join(l.Folder, name)
	} else {
		l.Path = filepath.Join(l.Path, name)
	}
	return l
}

// Parent returns the containing location and the row to reveal there.
func (l Location) Parent() (Location, string) {
	if l.InArchive() && l.Folder != "." {
		return Location{Path: l.Path, Folder: path.Dir(l.Folder)}, path.Base(l.Folder)
	}
	return Location{Path: filepath.Dir(l.Path)}, filepath.Base(l.Path)
}

// ResolveArchiveLocation recognizes displayed archive paths on the directory
// worker. Ordinary filesystem directories, including ones named *.zip, win.
func ResolveArchiveLocation(filename string) (Location, bool) {
	if info, err := os.Stat(filename); err == nil && info.IsDir() {
		return Location{}, false
	}
	for candidate := filename; ; candidate = filepath.Dir(candidate) {
		if ArchiveFormat(candidate) != nil {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				folder, err := filepath.Rel(candidate, filename)
				if err == nil {
					return Location{Path: candidate, Folder: filepath.ToSlash(folder)}, true
				}
			}
		}
		if parent := filepath.Dir(candidate); parent == candidate {
			return Location{}, false
		}
	}
}
