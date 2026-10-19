package app

import (
	"errors"
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	"github.com/dannyben/commando/internal/browser"
)

type archivePanelMove struct{ source, target browser.Location }

func (l *directoryLoader) captureArchiveTransfer(result *loadResult) {
	locations := result.request.transfer.archiveLocations
	paths := make([]string, len(locations))
	for i, location := range locations {
		paths[i] = location.Path
	}
	result.locations = resolveFileLocations(paths)
	result.archiveReconciled = true
}

func (l *directoryLoader) reportArchiveProgress(request loadRequest, text string) {
	if request.transfer.snapshot != nil || l.results == nil {
		return
	}
	l.results <- loadResult{request: request, progress: text}
	if l.invalidate != nil {
		l.invalidate()
	}
}

func (l *directoryLoader) refreshArchiveTransfer(result *loadResult) {
	locations := result.request.transfer.archiveLocations
	if !result.archiveReconciled {
		l.captureArchiveTransfer(result)
	}
	result.panelUpdates = make([]transferPanelUpdate, len(locations))
	for i, location := range locations {
		if location.Path == "" {
			continue
		}
		followed := false
		for _, move := range result.archiveMoves {
			suffix, inside := "", false
			if move.source.InArchive() {
				if !location.InArchive() || !samePath(result.locations[i], move.source.Path) {
					continue
				}
				inside = location.Folder == move.source.Folder || strings.HasPrefix(location.Folder, move.source.Folder+"/")
				suffix = strings.TrimPrefix(strings.TrimPrefix(location.Folder, move.source.Folder), "/")
			} else if !location.InArchive() {
				suffix, inside = movedLocationSuffix(move.source.Path, result.locations[i])
			}
			if !inside {
				continue
			}
			followed = true
			location = move.target
			if location.InArchive() {
				location.Folder = path.Join(location.Folder, filepath.ToSlash(suffix))
			} else {
				location.Path = filepath.Join(location.Path, filepath.FromSlash(suffix))
			}
			break
		}
		update := transferPanelUpdate{needed: true, followed: followed}
		if location.InArchive() {
			for {
				update.directory, update.err = browser.ReadArchive(location)
				if !errors.Is(update.err, fs.ErrNotExist) {
					break
				}
				if location.Folder == "." {
					update.directory, update.err = l.readExistingParent(filepath.Dir(location.Path))
					break
				}
				location.Folder = path.Dir(location.Folder)
			}
		} else {
			update.directory, update.err = l.readExistingParent(location.Path)
		}
		result.panelUpdates[i] = update
	}
}
