package browser

import (
	"os"
	"path/filepath"
)

// CreateFolder creates one named child without accepting paths or reusing an
// existing file, directory or link.
func CreateFolder(directory, name string) (Entry, error) {
	return createEntry(directory, name, true)
}

// CreateFile creates an empty file without replacing any existing entry.
func CreateFile(directory, name string) (Entry, error) {
	return createEntry(directory, name, false)
}

func createEntry(directory, name string, folder bool) (Entry, error) {
	if err := ValidateName(name); err != nil {
		return Entry{}, err
	}
	path := filepath.Join(directory, name)
	if folder {
		if err := os.Mkdir(path, 0777); err != nil {
			return Entry{}, err
		}
	} else {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
		if err != nil {
			return Entry{}, err
		}
		if err := file.Close(); err != nil {
			return Entry{}, err
		}
	}
	item := Entry{Name: name, Directory: folder, SizeKnown: !folder}
	if info, err := os.Stat(path); err == nil {
		item.Modified = info.ModTime()
	}
	return item, nil
}
