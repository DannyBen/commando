package browser

import (
	"os"
	"path/filepath"
	"slices"
	"time"
)

// Entry describes a file, folder, parent row, or drive. Path is the absolute
// navigation target for drives; ordinary directory entries use Name. Windows
// .lnk shortcuts remain files; only activation resolves their folder target.
type Entry struct {
	Name, Path        string
	Directory, Parent bool
	Hidden, System    bool // Windows attributes of this entry, independent of its name.
	Bytes             int64
	SizeKnown         bool
	Modified          time.Time // Zero when metadata is unavailable.
}

type Directory struct {
	Drives  bool
	Path    string
	Entries []Entry
}

// ReadDirectory reads one directory, including metadata, without recursing.
// An individual item's unavailable metadata must not hide the item itself.
func ReadDirectory(path string) (Directory, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return Directory{}, err
	}
	entries, err := os.ReadDir(absolute)
	if err != nil {
		return Directory{}, err
	}
	items := make([]Entry, 0, len(entries)+1)
	for _, de := range entries {
		item := Entry{Name: de.Name(), Directory: de.IsDir()}
		info, err := de.Info()
		if err == nil {
			setEntryAttributes(&item, info)
		}
		if de.Type()&os.ModeSymlink != 0 {
			// Follow folder links for navigation; retain broken links as entries.
			info, err = os.Stat(filepath.Join(absolute, de.Name()))
		}
		if err == nil {
			item.Directory = info.IsDir()
			item.Bytes, item.SizeKnown = info.Size(), info.Mode().IsRegular()
			item.Modified = info.ModTime()
		}
		items = append(items, item)
	}
	slices.SortFunc(items, CompareEntries)
	if filepath.Dir(absolute) != absolute {
		items = append([]Entry{{Name: "..", Directory: true, Parent: true}}, items...)
	}
	return Directory{Path: absolute, Entries: items}, nil
}
