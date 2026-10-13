package browser

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// CreateFolder creates one named child without accepting paths or reusing an
// existing file, directory or link.
func CreateFolder(directory, name string) (Entry, error) {
	return createEntry(directory, name, true, nil)
}

// CreateFile creates an empty file without replacing any existing entry.
func CreateFile(directory, name string) (Entry, error) {
	return CreateFileData(directory, name, nil)
}

// CreateFileData creates a file exclusively, preserving existing destinations.
func CreateFileData(directory, name string, data []byte) (Entry, error) {
	return createEntry(directory, name, false, data)
}

func createEntry(directory, name string, folder bool, data []byte) (Entry, error) {
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
			// Windows can report EISDIR for an existing folder even with O_EXCL.
			// It is still a name collision, and the existing entry stays intact.
			if info, statErr := os.Lstat(path); statErr == nil && info.IsDir() {
				return Entry{}, &os.PathError{Op: "create", Path: path, Err: fs.ErrExist}
			}
			return Entry{}, err
		}
		_, writeErr := file.Write(data)
		closeErr := file.Close()
		if err := errors.Join(writeErr, closeErr); err != nil {
			return Entry{}, errors.Join(err, os.Remove(path))
		}
	}
	item := Entry{Name: name, Directory: folder, SizeKnown: !folder}
	if info, err := os.Stat(path); err == nil {
		item.Modified = info.ModTime()
		item.Bytes = info.Size()
		setEntryAttributes(&item, info)
	}
	return item, nil
}
