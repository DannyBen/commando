package browser

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type archiveSource struct {
	path string
	info fs.FileInfo
}

func (p *ArchiveSources) Close() { p.close() }
func (p *ArchiveSources) Paths() []string {
	paths := make([]string, len(p.sources))
	for i, source := range p.sources {
		paths[i] = source.path
	}
	return paths
}

// Validate the entire tree again before removal, including new/removed entries.
// Per-item shell vetoes also reject replacements and changed file contents.
func (p *ArchiveSources) validate() error {
	if err := p.ValidateTarget(); err != nil {
		return err
	}
	current, err := planArchiveAdd(p.Paths(), p.target)
	if err != nil {
		return err
	}
	defer current.close()
	if len(current.files) != len(p.files) {
		return ErrArchiveSourceChanged
	}
	for i, before := range p.files {
		after := current.files[i]
		if before.name != after.name || before.info.Mode() != after.info.Mode() || !os.SameFile(before.info, after.info) || before.info.Size() != after.info.Size() || !before.info.ModTime().Equal(after.info.ModTime()) {
			return ErrArchiveSourceChanged
		}
	}
	for i, source := range p.sources {
		if !os.SameFile(source.info, current.sources[i].info) {
			return ErrArchiveSourceChanged
		}
	}
	return nil
}

func (p *ArchiveSources) validateItem(filename string) error {
	if err := p.ValidateTarget(); err != nil {
		return err
	}
	info, err := os.Lstat(filename)
	if err != nil {
		return err
	}
	before := p.items[transferPathKey(filename)]
	if before == nil || !os.SameFile(before, info) || before.Mode() != info.Mode() {
		return ErrArchiveSourceChanged
	}
	if !info.IsDir() && !sameArchiveInput(before, info) {
		return ErrArchiveSourceChanged
	}
	return nil
}

func (sources *ArchiveSources) prepareRemoval() error {
	if err := sources.validate(); err != nil {
		return err
	}
	paths := sources.Paths()
	parents := map[string]string{}
	for _, path := range paths {
		parents[filepath.Base(path)] = filepath.Dir(path)
	}
	sources.items = make(map[string]fs.FileInfo, len(sources.files))
	for _, file := range sources.files {
		top, _, _ := strings.Cut(file.name, "/")
		filename := filepath.Join(parents[top], filepath.FromSlash(file.name))
		sources.items[transferPathKey(filename)] = file.info
	}
	return nil
}

func RemoveArchiveSources(sources *ArchiveSources, owner uintptr) ([]string, error) {
	if err := sources.prepareRemoval(); err != nil {
		return nil, err
	}
	paths := sources.Paths()
	batch, err := deleteVerifiedPaths(paths, owner, sources.validateItem)
	var removed []string
	for i, path := range paths {
		if i < len(batch.completed) && batch.completed[i] {
			removed = append(removed, path)
		}
	}
	return removed, err
}

func (p *ArchiveSources) ValidateTarget() error {
	current, err := readArchiveClipboardStamp(p.target)
	if err != nil {
		return err
	}
	if current != p.targetStamp {
		return ErrArchiveChanged
	}
	return nil
}
