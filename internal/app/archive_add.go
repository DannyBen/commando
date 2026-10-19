package app

import (
	"errors"

	"github.com/dannyben/commando/internal/browser"
)

func (l *directoryLoader) archiveAddOperation(request loadRequest, result loadResult) loadResult {
	transfer := request.transfer
	// Clipboard paste and moves have separate tasks. This path owns a captured
	// panel-copy selection and must never consume or alter the file clipboard.
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
		stage, err = browser.StageArchive(transfer.archive.location, transfer.archive.names)
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
	l.refreshArchivePanels(&result, locations, target)
	return result
}
