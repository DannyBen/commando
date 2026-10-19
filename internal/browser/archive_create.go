package browser

import (
	"archive/zip"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// CreateArchive creates the final entry exclusively; missing parents are
// implied by its ZIP path. The shared rewrite commits the entire change at once.
func CreateArchive(location Location, name string, folder bool, data []byte) (Location, Entry, error) {
	adapter := ArchiveFormat(location.Path)
	if adapter == nil || !location.InArchive() || !fs.ValidPath(location.Folder) || !adapter.Capabilities().Add {
		return location, Entry{}, fs.ErrInvalid
	}
	normalized, err := NormalizeCreatePath(name)
	if err != nil {
		return location, Entry{}, err
	}
	return adapter.Create(location, filepath.ToSlash(normalized), folder, data)
}

func (zipAdapter) Create(location Location, name string, folder bool, data []byte) (Location, Entry, error) {
	fullName := path.Join(location.Folder, name)
	now := time.Now()
	err := rewriteZIP(location.Path, func(reader *zip.Reader, writer *zip.Writer) error {
		files, directories := map[string]bool{}, map[string]bool{".": true}
		for _, member := range reader.File {
			existing := strings.TrimSuffix(member.Name, "/")
			if !fs.ValidPath(existing) || existing == "." || strings.ContainsAny(existing, `\:`) {
				continue
			}
			if member.FileInfo().IsDir() {
				directories[existing] = true
			} else {
				files[existing] = true
			}
			for parent := path.Dir(existing); parent != "."; parent = path.Dir(parent) {
				directories[parent] = true
			}
		}
		if !directories[location.Folder] {
			return fs.ErrNotExist
		}
		if files[fullName] || directories[fullName] {
			return fs.ErrExist
		}
		for parent := path.Dir(fullName); parent != "."; parent = path.Dir(parent) {
			if files[parent] {
				return fs.ErrExist
			}
		}
		for _, member := range reader.File {
			if err := writer.Copy(member); err != nil {
				return err
			}
		}
		header := &zip.FileHeader{Name: fullName, Method: zip.Deflate, Modified: now}
		header.SetMode(0644)
		if folder {
			header.Name += "/"
			header.Method = zip.Store
			header.SetMode(os.ModeDir | 0755)
		}
		output, err := writer.CreateHeader(header)
		if err != nil {
			return err
		}
		if !folder {
			_, err = output.Write(data)
		}
		return err
	})
	if err != nil {
		return location, Entry{}, err
	}
	parent := Location{Path: location.Path, Folder: path.Dir(fullName)}
	return parent, Entry{Name: path.Base(fullName), Directory: folder, SizeKnown: !folder, Bytes: int64(len(data)), Modified: now}, nil
}
