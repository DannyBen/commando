package browser

import (
	"archive/zip"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	members, tops, err := planZIPExtraction(&r.Reader, location, names)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	return extractZIPMembers(root, members, tops)
}

type zipExtractMember struct {
	file *zip.File
	name string
}

// Validate the complete selection before writing any output. A nil selection
// means the entire archive, including entries hidden from ordinary browsing.
func planZIPExtraction(reader *zip.Reader, location Location, names []string) ([]zipExtractMember, map[string]bool, error) {
	selected := map[string]bool{}
	for _, name := range names {
		selected[name] = false
	}
	prefix := ""
	if location.Folder != "." {
		prefix = location.Folder + "/"
	}
	plan := newArchiveExtractPlan()
	var members []zipExtractMember
	for _, member := range reader.File {
		if !strings.HasPrefix(member.Name, prefix) {
			continue
		}
		relative := strings.TrimSuffix(strings.TrimPrefix(member.Name, prefix), "/")
		child, _, _ := strings.Cut(relative, "/")
		if names != nil {
			if _, ok := selected[child]; !ok {
				continue
			}
		}
		if err := plan.add(relative, member.Mode()); err != nil {
			return nil, nil, err
		}
		selected[child] = true
		members = append(members, zipExtractMember{member, filepath.FromSlash(relative)})
	}
	for _, found := range selected {
		if !found {
			return nil, nil, fs.ErrNotExist
		}
	}
	return members, plan.tops, nil
}

type archiveExtractPlan struct {
	spellings                   map[string]string
	directories, explicit, tops map[string]bool
}

func newArchiveExtractPlan() archiveExtractPlan {
	return archiveExtractPlan{spellings: map[string]string{}, directories: map[string]bool{}, explicit: map[string]bool{}, tops: map[string]bool{}}
}

func (p *archiveExtractPlan) add(name string, mode fs.FileMode) error {
	if !fs.ValidPath(name) || name == "." {
		return ErrInvalidName
	}
	if !mode.IsRegular() && !mode.IsDir() {
		return errors.ErrUnsupported
	}
	if p.explicit[name] {
		return fs.ErrExist
	}
	p.explicit[name] = true
	components := strings.Split(name, "/")
	for i, component := range components {
		if err := ValidateName(component); err != nil {
			return err
		}
		name := strings.Join(components[:i+1], "/")
		key := strings.ToUpper(name)
		directory := i < len(components)-1 || mode.IsDir()
		if previous, ok := p.spellings[key]; ok && (previous != name || p.directories[key] != directory) {
			return fs.ErrExist
		}
		p.spellings[key], p.directories[key] = name, directory
	}
	p.tops[components[0]] = len(components) > 1 || mode.IsDir()
	return nil
}

func prepareArchiveExtraction(root *os.Root, tops map[string]bool) error {
	// Refuse existing roots, including case variants on development hosts. Never
	// merge an archive directory into an existing filesystem directory.
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return err
	}
	existing := make(map[string]bool, len(entries))
	for _, entry := range entries {
		existing[strings.ToUpper(entry.Name())] = true
	}
	for name := range tops {
		if existing[strings.ToUpper(name)] {
			return fs.ErrExist
		}
	}
	for name, directory := range tops {
		if directory {
			if err := root.Mkdir(name, 0700); err != nil {
				return err
			}
		}
	}
	return nil
}

func extractZIPMembers(root *os.Root, members []zipExtractMember, tops map[string]bool) error {
	if err := prepareArchiveExtraction(root, tops); err != nil {
		return err
	}
	for _, member := range members {
		if member.file.Mode().IsDir() {
			if err := root.MkdirAll(member.name, 0700); err != nil {
				return err
			}
		} else {
			if err := root.MkdirAll(filepath.Dir(member.name), 0700); err != nil {
				return err
			}
			if err := extractZipFile(root, member.name, member.file); err != nil {
				return err
			}
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
	return extractArchiveFile(root, filename, source, member.Modified)
}

func extractArchiveFile(root *os.Root, filename string, source io.Reader, modified time.Time) error {
	target, err := root.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	// Stream payloads and propagate reader errors without buffering whole files.
	_, copyErr := io.Copy(target, source)
	closeErr := target.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return err
	}
	if !modified.IsZero() {
		return root.Chtimes(filename, modified, modified)
	}
	return nil
}
