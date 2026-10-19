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
	result.panelUpdates = make([]transferPanelUpdate, len(locations))
	for i, location := range locations {
		if location.Path == "" || result.locations[i] == "" {
			continue
		}
		update := transferPanelUpdate{needed: true}
		if location.InArchive() && samePath(result.locations[i], original) {
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
	return result
}

func (v *view) applyArchiveRemovalPanels(result loadResult) (failed bool) {
	locations := result.request.removal.archiveLocations
	for i, location := range locations {
		if i >= len(v.panels) || i >= len(result.panelUpdates) {
			continue
		}
		p, update := &v.panels[i], result.panelUpdates[i]
		if !update.needed || p.drives || !samePath(p.path, location.Path) || p.archiveFolder != location.Folder {
			continue
		}
		if update.err != nil {
			failed = true
			continue
		}
		p.reconcileDirectoryListing(update.directory)
		p.refreshNeeded = false
	}
	return failed
}
