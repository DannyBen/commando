package app

import (
	"errors"
	"path"
	"path/filepath"
	"slices"

	"github.com/dannyben/commando/internal/browser"
)

func (l *directoryLoader) finishArchiveMove(result loadResult, snapshot browser.FileClipboard) loadResult {
	result.transferMove = true
	if snapshot.Archive != nil && len(result.movedNames) > 0 && result.request.transfer.snapshot == nil {
		result.clipboard, result.clipboardErr = l.completeClipboard(snapshot, result.movedNames)
	} else if snapshot.Archive == nil && len(result.movedSources) > 0 && result.request.transfer.snapshot == nil {
		result.clipboard, result.clipboardErr = l.completeClipboard(snapshot, result.movedSources)
	}
	l.refreshArchiveTransfer(&result)
	return result
}

func (l *directoryLoader) moveArchiveOut(request loadRequest, result loadResult, snapshot browser.FileClipboard) loadResult {
	l.captureArchiveTransfer(&result)
	selection := request.transfer.archive
	if selection.reference == nil {
		var err error
		selection.reference, err = browser.CaptureArchiveClipboard(selection.location, selection.names)
		if err != nil {
			result.err = err
			return result
		}
	}
	stage, err := selection.stage()
	if err != nil {
		result.err = err
		return result
	}
	result.transferred, result.err = l.transfer(browser.TransferRequest{Paths: stage.Paths, Directory: request.path, Move: true, TemporarySources: true, Owner: request.transfer.owner, OpenFolders: result.locations})
	var confirmed []string
	targets := map[string]string{}
	stageNames := make(map[string]string, len(stage.Paths))
	for i, source := range stage.Paths {
		stageNames[transferNameKey(source)] = selection.names[i]
	}
	for _, item := range result.transferred.Items {
		if !item.Moved {
			continue
		}
		name, ok := stageNames[transferNameKey(item.Source)]
		if !ok {
			continue
		}
		confirmed = append(confirmed, name)
		targets[name] = filepath.Join(result.transferred.Directory, item.Entry.Name)
	}
	stage.Close()
	result.transferred.Summary.Partial += max(0, result.transferred.Summary.Completed-len(confirmed))
	result.transferred.Summary.Completed = 0
	if len(confirmed) > 0 {
		if err := selection.reference.Remove(confirmed); err != nil {
			result.retainedCopies = len(confirmed)
			result.transferred.Summary.Unfinished += len(confirmed)
			result.err = errors.Join(result.err, err)
			result.transferred.Summary.Completed = 0
		} else {
			result.movedNames = confirmed
			result.transferred.Summary.Completed = len(confirmed)
			for _, name := range confirmed {
				result.archiveMoves = append(result.archiveMoves, archivePanelMove{source: browser.Location{Path: selection.reference.Location.Path, Folder: path.Join(selection.location.Folder, name)}, target: browser.Location{Path: targets[name]}})
			}
		}
	}
	return l.finishArchiveMove(result, snapshot)
}

func (l *directoryLoader) moveIntoArchive(request loadRequest, result loadResult, snapshot browser.FileClipboard) loadResult {
	l.captureArchiveTransfer(&result)
	target, err := resolveFileLocation(request.path)
	if err != nil {
		result.err = err
		return result
	}
	destination := browser.Location{Path: target, Folder: request.archiveFolder}
	selection := request.transfer.archive
	var stage *browser.ArchiveStage
	sources := snapshot.Paths
	if selection != nil {
		if selection.reference == nil {
			selection.reference, err = browser.CaptureArchiveClipboard(selection.location, selection.names)
			if err != nil {
				result.err = err
				return result
			}
		}
		if samePath(selection.reference.Location.Path, target) {
			if selection.location.Folder == destination.Folder {
				result.alreadyHere = true
				return result
			}
			result.err = browser.MoveArchive(selection.reference, destination)
			if result.err == nil {
				result.movedNames = slices.Clone(selection.names)
			}
		} else {
			stage, err = selection.stage()
			if err != nil {
				result.err = err
				return result
			}
			sources = stage.Paths
			result.err = browser.AddArchive(destination, sources)
			stage.Close()
			if result.err == nil {
				result.err = selection.reference.Remove(selection.names)
				if result.err != nil {
					result.retainedCopies = len(selection.names)
				} else {
					result.movedNames = slices.Clone(selection.names)
				}
			}
		}
		for _, name := range result.movedNames {
			result.archiveMoves = append(result.archiveMoves, archivePanelMove{source: browser.Location{Path: selection.reference.Location.Path, Folder: path.Join(selection.location.Folder, name)}, target: destination.Child(name)})
		}
	} else {
		receipt, addErr := browser.AddArchiveSources(destination, sources)
		result.err = addErr
		if receipt != nil {
			removed, removeErr := l.removeArchiveSources(receipt, request.transfer.owner)
			paths := receipt.Paths()
			receipt.Close()
			result.err = errors.Join(result.err, removeErr)
			result.retainedCopies = len(paths) - len(removed)
			removedPaths := make(map[string]bool, len(removed))
			for _, source := range removed {
				removedPaths[transferNameKey(source)] = true
			}
			for i, source := range paths {
				if !removedPaths[transferNameKey(source)] {
					continue
				}
				result.movedSources = append(result.movedSources, snapshot.Paths[i])
				result.archiveMoves = append(result.archiveMoves, archivePanelMove{source: browser.Location{Path: source}, target: destination.Child(filepath.Base(source))})
			}
		}
	}
	count := len(result.movedNames) + len(result.movedSources)
	requested := len(sources)
	if selection != nil {
		requested = len(selection.names)
	}
	result.transferred = browser.TransferResult{Directory: target, Attempted: count > 0 || result.retainedCopies > 0, Summary: browser.TransferSummary{Requested: requested, Completed: count, Unfinished: requested - count}}
	for _, move := range result.archiveMoves {
		result.transferred.Items = append(result.transferred.Items, browser.TransferredItem{Entry: browser.Entry{Name: path.Base(move.target.Folder)}, Moved: true})
	}
	return l.finishArchiveMove(result, snapshot)
}
