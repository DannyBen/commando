package browser

import (
	"archive/zip"
	"os"
)

// UnpackZIP streams a whole ZIP into a filesystem directory. A nonempty name
// creates an exclusive subfolder first. Once extraction starts, errors may leave
// partial output; the source archive and preexisting entries are never changed.
func UnpackZIP(filename, directory, name string) (Entry, error) {
	if name != "" {
		if err := ValidateName(name); err != nil {
			return Entry{}, err
		}
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
		return created, extractZIPMembers(child, members, tops)
	}
	return created, extractZIPMembers(root, members, tops)
}
