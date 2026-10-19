package browser

import (
	"archive/zip"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dannyben/commando/internal/tempfiles"
)

// ArchiveStage contains only disposable files owned by this copy operation.
// Keep it alive until the native transfer finishes, then close it on every exit.
type ArchiveStage struct {
	Directory string
	Paths     []string
}

func (s *ArchiveStage) Close() error { return tempfiles.Remove(s.Directory) }

func StageArchive(location Location, names []string) (*ArchiveStage, error) {
	return stageArchive(location, names, tempfiles.NewSession)
}

func stageArchive(location Location, names []string, newDirectory func() (string, error)) (*ArchiveStage, error) {
	adapter := ArchiveFormat(location.Path)
	if !location.InArchive() || !fs.ValidPath(location.Folder) || adapter == nil || !adapter.Capabilities().Read || len(names) == 0 {
		return nil, fs.ErrInvalid
	}
	seen := map[string]bool{}
	for _, name := range names {
		if err := ValidateName(name); err != nil {
			return nil, err
		}
		key := strings.ToUpper(name)
		if seen[key] {
			return nil, fs.ErrExist
		}
		seen[key] = true
	}
	directory, err := newDirectory()
	if err != nil {
		return nil, err
	}
	stage := &ArchiveStage{Directory: directory}
	if err := adapter.Extract(location, names, directory); err != nil {
		stage.Close()
		return nil, err
	}
	for _, name := range names {
		stage.Paths = append(stage.Paths, filepath.Join(directory, name))
	}
	return stage, nil
}

func (zipAdapter) Extract(location Location, names []string, directory string) error {
	r, err := zip.OpenReader(location.Path)
	if err != nil {
		if r != nil {
			r.Close()
		}
		return err
	}
	defer r.Close()
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	selected := map[string]bool{}
	for _, name := range names {
		selected[name] = false
	}
	prefix := ""
	if location.Folder != "." {
		prefix = location.Folder + "/"
	}
	// Track every component, including implied directories, with Windows identity
	// rules even on development hosts. Never merge case-distinct archive names.
	spellings := map[string]string{}
	for _, member := range r.File {
		if !strings.HasPrefix(member.Name, prefix) {
			continue
		}
		relative := strings.TrimSuffix(strings.TrimPrefix(member.Name, prefix), "/")
		child, _, _ := strings.Cut(relative, "/")
		if _, ok := selected[child]; !ok {
			continue
		}
		if !fs.ValidPath(relative) || relative == "." {
			return ErrInvalidName
		}
		components := strings.Split(relative, "/")
		for i, component := range components {
			if err := ValidateName(component); err != nil {
				return err
			}
			name := strings.Join(components[:i+1], "/")
			key := strings.ToUpper(name)
			if previous, ok := spellings[key]; ok && previous != name {
				return fs.ErrExist
			}
			spellings[key] = name
		}
		mode := member.Mode()
		if !mode.IsRegular() && !mode.IsDir() {
			return errors.ErrUnsupported
		}
		selected[child] = true
		native := filepath.FromSlash(relative)
		if mode.IsDir() {
			if err := root.MkdirAll(native, 0700); err != nil {
				return err
			}
			continue
		}
		if err := root.MkdirAll(filepath.Dir(native), 0700); err != nil {
			return err
		}
		if err := extractZipFile(root, native, member); err != nil {
			return err
		}
	}
	for _, found := range selected {
		if !found {
			return fs.ErrNotExist
		}
	}
	return nil
}

func extractZipFile(root *os.Root, filename string, member *zip.File) error {
	source, err := member.Open()
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := root.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	// Streaming retains ZIP checksum verification without loading members into RAM.
	_, copyErr := io.Copy(target, source)
	closeErr := target.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return err
	}
	if !member.Modified.IsZero() {
		return root.Chtimes(filename, member.Modified, member.Modified)
	}
	return nil
}
