package app

import (
	"errors"
	"io/fs"
	"path"
	"path/filepath"

	"github.com/dannyben/commando/internal/browser"
)

func (l *directoryLoader) archiveRemovalOperation(request loadRequest) loadResult {
	result := loadResult{request: request}
	locations := request.removal.archiveLocations
	paths := make([]string, len(locations))
	for i, location := range locations {
		paths[i] = location.Path
	}
	result.locations = resolveFileLocations(paths)
	original, err := resolveFileLocation(request.path)
	if err != nil {
		result.err = err
		return result
	}
	result.removed, result.err = browser.DeleteArchive(browser.Location{Path: original, Folder: request.archiveFolder}, request.removal.names)
	if result.err != nil {
		return result
	}
	l.refreshArchivePanels(&result, locations, original)
	return result
}

func (l *directoryLoader) refreshArchivePanels(result *loadResult, locations []browser.Location, original string) {
	result.panelUpdates = make([]transferPanelUpdate, len(locations))
	for i, location := range locations {
		if location.Path == "" || result.locations[i] == "" {
			continue
		}
		update := transferPanelUpdate{needed: true}
		if location.InArchive() && samePath(result.locations[i], original) {
			if result.request.rename != nil {
				location.Folder = renamedArchiveFolder(location.Folder, result.request)
			}
			for {
				update.directory, update.err = browser.ReadArchive(location)
				if !errors.Is(update.err, fs.ErrNotExist) || location.Folder == "." {
					break
				}
				location.Folder = path.Dir(location.Folder)
			}
		} else if !location.InArchive() && samePath(result.locations[i], filepath.Dir(original)) {
			update.directory, update.err = l.readDirectory(location.Path)
		} else {
			continue
		}
		result.panelUpdates[i] = update
	}
}

func (v *view) applyArchivePanels(result loadResult, locations []browser.Location) (failed bool) {
	for i, location := range locations {
		if i >= len(v.panels) || i >= len(result.panelUpdates) {
			continue
		}
		p, update := &v.panels[i], result.panelUpdates[i]
		if !update.needed || p.drives || !samePath(p.path, location.Path) || p.archiveFolder != location.Folder {
			continue
		}
		prefix, prefixAt := p.prefix, p.prefixAt
		if rename := result.request.rename; rename != nil && location.InArchive() && i < len(result.locations) && samePath(result.locations[i], result.mutationDirectory) {
			if p.archiveFolder == result.request.archiveFolder {
				p.renameEntry(rename.oldName, rename.newName)
			}
			p.archiveFolder = renamedArchiveFolder(p.archiveFolder, result.request)
		}
		if update.err != nil {
			p.prefix, p.prefixAt = prefix, prefixAt
			p.refreshNeeded = true
			failed = true
			continue
		}
		unchanged := p.sameLocation(update.directory)
		p.reconcileDirectoryListing(update.directory)
		if unchanged {
			p.prefix, p.prefixAt = prefix, prefixAt
		}
		p.refreshNeeded = false
	}
	return failed
}

func (v *view) archiveOperationLocations() []browser.Location {
	locations := make([]browser.Location, len(v.panels))
	for i := range v.panels {
		if !v.panels[i].drives {
			locations[i] = v.panels[i].location()
		}
	}
	return locations
}
