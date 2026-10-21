package browser

import (
	"archive/zip"
	"errors"
	"os"
	"strings"
)

// UnpackExtension identifies extraction formats independently of browsing support.
func UnpackExtension(filename string) string {
	filename = strings.ToLower(filename)
	for _, extension := range []string{".tar.gz", ".tgz", ".tar", ".zip"} {
		if strings.HasSuffix(filename, extension) {
			return extension
		}
	}
	return ""
}

// UnpackArchive streams an archive into a filesystem directory. A nonempty name
// creates an exclusive subfolder first. Once extraction starts, errors may leave
// partial output; the source archive and preexisting entries are never changed.
func UnpackArchive(filename, directory, name string) (Entry, error) {
	if name != "" {
		if err := ValidateName(name); err != nil {
			return Entry{}, err
		}
	}
	extension := UnpackExtension(filename)
	if extension == ".tar" || extension == ".tar.gz" || extension == ".tgz" {
		return unpackTAR(filename, directory, name, extension != ".tar")
	}
	if extension != ".zip" {
		return Entry{}, errors.ErrUnsupported
	}
	r, err := zip.OpenReader(filename)
	if err != nil {
		if r != nil {
			r.Close()
		}
		return Entry{}, err
	}
	defer r.Close()
	members, tops, err := planZIPExtraction(&r.Reader, Location{Path: filename, Folder: "."}, nil)
	if err != nil {
		return Entry{}, err
	}
	return unpackDirectory(directory, name, func(root *os.Root) error {
		return extractZIPMembers(root, members, tops)
	})
}

func unpackDirectory(directory, name string, extract func(*os.Root) error) (Entry, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return Entry{}, err
	}
	defer root.Close()
	var created Entry
	if name != "" {
		if err := root.Mkdir(name, 0777); err != nil {
			return Entry{}, err
		}
		created = Entry{Name: name, Directory: true}
		if info, err := root.Stat(name); err == nil {
			created.Modified, created.Created = info.ModTime(), creationTime(info)
			setEntryAttributes(&created, info)
		}
		child, err := root.OpenRoot(name)
		if err != nil {
			return created, err
		}
		defer child.Close()
		return created, extract(child)
	}
	return created, extract(root)
}
