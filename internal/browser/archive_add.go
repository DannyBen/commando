package browser

import (
	"archive/zip"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var ErrArchiveSourceChanged = errors.New("source changed while creating archive")
var ErrArchiveSelfCopy = errors.New("archive cannot contain itself")

type archiveInput struct {
	root        *os.Root
	local, name string
	info        fs.FileInfo
}

type ArchiveSources struct {
	targetStamp archiveClipboardStamp
	items       map[string]fs.FileInfo
	sources     []archiveSource
	target      string
	roots       []*os.Root
	files       []archiveInput
	names       map[string]bool
}

func (p *ArchiveSources) close() {
	for _, root := range p.roots {
		root.Close()
	}
}

func AddArchive(location Location, paths []string) error {
	sources, err := AddArchiveSources(location, paths)
	if sources != nil {
		sources.close()
	}
	return err
}

func AddArchiveSources(location Location, paths []string) (*ArchiveSources, error) {
	adapter := ArchiveFormat(location.Path)
	if adapter == nil || !location.InArchive() || !fs.ValidPath(location.Folder) || !adapter.Capabilities().Add || len(paths) == 0 {
		return nil, fs.ErrInvalid
	}
	return adapter.Add(location, paths)
}

func (zipAdapter) Add(location Location, paths []string) (*ArchiveSources, error) {
	target, err := ResolveOperationPath(location.Path)
	if err != nil {
		return nil, err
	}
	target, err = filepath.EvalSymlinks(target)
	if err != nil {
		return nil, err
	}
	var sources *ArchiveSources
	err = rewriteZIP(target, func(reader *zip.Reader, writer *zip.Writer) error {
		plan, err := planArchiveAdd(paths, target)
		if err != nil {
			return err
		}
		sources = &plan
		prefix, found := "", location.Folder == "."
		if !found {
			prefix = location.Folder + "/"
		}
		for _, file := range reader.File {
			name := strings.TrimSuffix(file.Name, "/")
			if name == location.Folder && file.FileInfo().IsDir() {
				found = true
			}
			if !strings.HasPrefix(file.Name, prefix) {
				continue
			}
			if fs.ValidPath(name) && !strings.ContainsAny(name, `\:`) {
				found = true
			}
			child, _, _ := strings.Cut(strings.TrimPrefix(file.Name, prefix), "/")
			if plan.names[child] {
				return fs.ErrExist
			}
		}
		if !found {
			return fs.ErrNotExist
		}
		for _, file := range reader.File {
			if err := writer.Copy(file); err != nil {
				return err
			}
		}
		for _, file := range plan.files {
			file.name = path.Join(location.Folder, file.name)
			if err := writeArchiveInput(writer, file); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		sources.targetStamp, err = readArchiveClipboardStamp(target)
	}
	if err != nil {
		if sources != nil {
			sources.close()
		}
		return nil, err
	}
	return sources, nil
}

// Anchor each selected directory itself, rather than following paths from its
// parent. Walks and later reads remain confined even if source paths change.
func planArchiveAdd(paths []string, target string) (plan ArchiveSources, err error) {
	plan.names = map[string]bool{}
	plan.target = target
	defer func() {
		if err != nil {
			plan.close()
		}
	}()
	targetInfo, err := os.Stat(target)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return plan, err
	}
	for _, filename := range paths {
		if !filepath.IsAbs(filename) {
			return plan, fs.ErrInvalid
		}
		resolved, err := ResolveOperationPath(filename)
		if err != nil {
			return plan, err
		}
		parent, err := filepath.EvalSymlinks(filepath.Dir(resolved))
		if err != nil {
			return plan, err
		}
		name := filepath.Base(resolved)
		if err := ValidateName(name); err != nil {
			return plan, err
		}
		if plan.names[name] {
			return plan, fs.ErrExist
		}
		plan.names[name] = true
		root, err := os.OpenRoot(parent)
		if err != nil {
			return plan, err
		}
		info, err := root.Lstat(name)
		if err != nil {
			root.Close()
			return plan, err
		}
		if info.Mode().Type()&^os.ModeDir != 0 {
			root.Close()
			return plan, errors.ErrUnsupported
		}
		if targetInfo != nil && os.SameFile(info, targetInfo) || info.IsDir() && pathWithin(filepath.Join(parent, name), target) {
			root.Close()
			return plan, ErrArchiveSelfCopy
		}
		plan.sources = append(plan.sources, archiveSource{path: filepath.Join(parent, name), info: info})
		if !info.IsDir() {
			plan.roots = append(plan.roots, root)
			plan.files = append(plan.files, archiveInput{root: root, local: name, name: name, info: info})
			continue
		}
		anchored, err := root.OpenRoot(name)
		root.Close()
		if err != nil {
			return plan, err
		}
		plan.roots = append(plan.roots, anchored)
		current, err := anchored.Stat(".")
		if err != nil {
			return plan, err
		}
		if !os.SameFile(info, current) {
			return plan, ErrArchiveSourceChanged
		}
		err = fs.WalkDir(anchored.FS(), ".", func(local string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if local != "." {
				if err := ValidateName(entry.Name()); err != nil {
					return err
				}
			}
			info, err := anchored.Lstat(local)
			if err != nil {
				return err
			}
			if info.Mode().Type()&^os.ModeDir != 0 {
				return errors.ErrUnsupported
			}
			if targetInfo != nil && os.SameFile(info, targetInfo) {
				return ErrArchiveSelfCopy
			}
			plan.files = append(plan.files, archiveInput{root: anchored, local: local, name: path.Join(name, local), info: info})
			return nil
		})
		if err != nil {
			return plan, err
		}
	}
	return plan, nil
}

func writeArchiveInput(writer *zip.Writer, file archiveInput) error {
	header, err := zip.FileInfoHeader(file.info)
	if err != nil {
		return err
	}
	header.Name = file.name
	if file.info.IsDir() {
		header.Name += "/"
		_, err := writer.CreateHeader(header)
		return err
	}
	source, err := file.root.Open(file.local)
	if err != nil {
		return err
	}
	defer source.Close()
	current, err := source.Stat()
	if err != nil {
		return err
	}
	if !sameArchiveInput(file.info, current) {
		return ErrArchiveSourceChanged
	}
	header.Method = zip.Deflate
	output, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	count, err := io.Copy(output, io.LimitReader(source, file.info.Size()))
	if err != nil {
		return err
	}
	after, err := source.Stat()
	if err != nil {
		return err
	}
	if count != file.info.Size() || !sameArchiveInput(file.info, after) {
		return ErrArchiveSourceChanged
	}
	return source.Close()
}

func sameArchiveInput(before, after fs.FileInfo) bool {
	return after.Mode().IsRegular() && os.SameFile(before, after) && before.Size() == after.Size() && before.ModTime().Equal(after.ModTime())
}
