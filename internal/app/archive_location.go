package app

import (
	"path/filepath"

	"github.com/dannyben/commando/internal/browser"
)

func (p *panel) location() browser.Location {
	return browser.Location{Path: p.path, Folder: p.archiveFolder}
}

func (p *panel) nativeFolder() bool {
	return !p.drives && p.location().Capabilities().NativePaths
}

func (p *panel) entryKey(name string) string {
	if p.archiveFolder != "" {
		return name // Archive names remain case-sensitive, including on Windows.
	}
	return transferNameKey(name)
}

func (p *panel) sameLocation(directory browser.Directory) bool {
	return p.drives == directory.Drives && p.archiveFolder == directory.ArchiveFolder &&
		(p.drives || samePath(filepath.Clean(p.path), filepath.Clean(directory.Path)))
}

func (v *view) loadLocation(index int, location browser.Location, selected string) {
	v.queueLoad(loadRequest{panel: index, path: location.Path, archiveFolder: location.Folder, selectName: selected})
}

func openReady(v *view) bool {
	if !selectedReady(v) {
		return false
	}
	p := &v.panels[v.active]
	return p.nativeFolder() || p.drives || p.items[p.selected].file.Directory
}

// Adjacent-panel navigation and Enter use the same location transitions.
func (p *panel) selectedLocation() (browser.Location, string) {
	file := p.items[p.selected].file
	if file.Parent {
		return p.location().Parent()
	}
	if file.Path != "" {
		return browser.Location{Path: file.Path}, ""
	}
	if file.Directory {
		return p.location().Child(file.Name), ""
	}
	if p.nativeFolder() && browser.ArchiveFormat(file.Name) != nil {
		return browser.Location{Path: filepath.Join(p.path, file.Name), Folder: "."}, ""
	}
	return p.location(), file.Name
}
