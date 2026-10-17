package browser

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// NormalizeCreatePath validates every component before any directory is created.
// Both separators are accepted; absolute paths and traversal are not names.
func NormalizeCreatePath(name string) (string, error) {
	parts := strings.Split(strings.ReplaceAll(name, `\`, "/"), "/")
	for _, part := range parts {
		if err := ValidateName(part); err != nil {
			return "", err
		}
	}
	return filepath.Join(parts...), nil
}

// CreatePath makes missing parents, then exclusively creates the final item.
// Failures may leave parent directories behind; a retry reuses those parents.
func CreatePath(directory, name string, folder bool, data []byte) (string, Entry, error) {
	name, err := NormalizeCreatePath(name)
	if err != nil {
		return directory, Entry{}, err
	}
	parent := filepath.Join(directory, filepath.Dir(name))
	if filepath.Dir(name) != "." {
		if err := os.MkdirAll(parent, 0777); err != nil {
			return parent, Entry{}, err
		}
	}
	item, err := createEntry(parent, filepath.Base(name), folder, data)
	return parent, item, err
}

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
		item.Created = creationTime(info)
		item.Bytes = info.Size()
		setEntryAttributes(&item, info)
	}
	return item, nil
}
