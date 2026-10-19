package browser

import (
	"archive/zip"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// PackZIP creates a new ZIP exclusively, streaming through the same rooted,
// checked source reads as AddArchive. Sources are never modified.
func PackZIP(directory, name string, paths []string) (entry Entry, err error) {
	if err := ValidateName(name); err != nil {
		return entry, err
	}
	if len(paths) == 0 {
		return entry, fs.ErrInvalid
	}
	parent, err := ResolveOperationPath(directory)
	if err != nil {
		return entry, err
	}
	parent, err = filepath.EvalSymlinks(parent)
	if err != nil {
		return entry, err
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		return entry, err
	}
	defer root.Close()
	if _, err := root.Lstat(name); err == nil {
		return entry, fs.ErrExist
	} else if !errors.Is(err, fs.ErrNotExist) {
		return entry, err
	}
	plan, err := planArchiveAdd(paths, filepath.Join(parent, name))
	if err != nil {
		return entry, err
	}
	defer plan.close()
	output, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0666)
	if err != nil {
		return entry, err
	}
	writer := zip.NewWriter(output)
	for _, file := range plan.files {
		if err = writeArchiveInput(writer, file); err != nil {
			break
		}
	}
	err = errors.Join(err, writer.Close(), output.Sync())
	info, statErr := output.Stat()
	err = errors.Join(err, statErr, output.Close())
	if err != nil {
		// Remove only our own incomplete output, never an external replacement.
		if current, statErr := root.Lstat(name); statErr == nil && info != nil && os.SameFile(info, current) {
			err = errors.Join(err, root.Remove(name))
		}
		return entry, err
	}
	entry = Entry{Name: name, SizeKnown: true, Bytes: info.Size(), Modified: info.ModTime(), Created: creationTime(info)}
	setEntryAttributes(&entry, info)
	return entry, nil
}
