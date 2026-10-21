package app

import (
	"errors"
	"path/filepath"

	"github.com/dannyben/commando/internal/browser"
)

func (l *directoryLoader) archiveAddOperation(request loadRequest, result loadResult) loadResult {
	transfer := request.transfer
	// The worker supplies a captured selection. Archive writes never alter the
	// clipboard; moves and other mutation types remain separate tasks.
	if transfer.snapshot == nil || transfer.snapshot.Move || transfer.shortcuts || transfer.duplicate {
		result.err = errors.ErrUnsupported
		return result
	}
	locations := transfer.archiveLocations
	paths := make([]string, len(locations))
	for i, location := range locations {
		paths[i] = location.Path
	}
	result.locations = resolveFileLocations(paths)
	target, err := resolveFileLocation(request.path)
	if err != nil {
		result.err = err
		return result
	}
	sources := transfer.snapshot.Paths
	var stage *browser.ArchiveStage
	if transfer.archive != nil {
		stage, err = transfer.archive.stage()
		if err != nil {
			result.err = err
			return result
		}
		sources = stage.Paths
	}
	result.err = browser.AddArchive(browser.Location{Path: target, Folder: request.archiveFolder}, sources)
	if stage != nil {
		stage.Close()
	}
	if result.err != nil {
		return result
	}
	result.transferred = browser.TransferResult{Directory: target, Attempted: true, Summary: browser.TransferSummary{Requested: len(sources), Completed: len(sources)}}
	for _, source := range sources {
		result.transferred.Items = append(result.transferred.Items, browser.TransferredItem{Entry: browser.Entry{Name: filepath.Base(source)}})
	}
	l.refreshArchivePanels(&result, locations, target)
	return result
}
