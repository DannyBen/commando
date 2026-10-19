package app

import (
	"path/filepath"

	"github.com/dannyben/commando/internal/browser"
)

func (l *directoryLoader) fileOperation(request loadRequest) loadResult {
	result := loadResult{request: request}
	locations := []string(nil)
	if request.clipboard != nil {
		locations = request.clipboard.locations
	} else {
		locations = request.transfer.locations
	}
	result.locations = resolveFileLocations(locations)
	if capture := request.clipboard; capture != nil {
		paths := make([]string, 0, len(capture.paths))
		for _, path := range capture.paths {
			resolved, err := browser.ResolveOperationPath(path)
			if err != nil {
				result.err = err
				return result
			}
			parent, err := filepath.EvalSymlinks(filepath.Dir(resolved))
			if err != nil {
				result.err = err
				return result
			}
			paths = append(paths, filepath.Join(parent, filepath.Base(resolved)))
		}
		result.clipboard, result.err = l.writeClipboard(paths, capture.move)
		return result
	}
	if request.archiveFolder != "" {
		return l.archiveAddOperation(request, result)
	}
	if request.transfer.archive != nil {
		return l.archiveCopyOperation(request, result)
	}
	var snapshot browser.FileClipboard
	if request.transfer.snapshot != nil {
		snapshot = *request.transfer.snapshot
	} else {
		snapshot, result.err = l.readClipboard()
	}
	if result.err != nil {
		return result
	}
	if !request.transfer.shortcuts && len(snapshot.ImagePNG) > 0 && len(snapshot.Paths) == 0 {
		result.clipboard = snapshot
		return result
	}
	if len(snapshot.Paths) == 0 {
		result.err = browser.ErrNoClipboardFiles
		return result
	}
	// File selections take precedence over any alternate image representation.
	snapshot.ImagePNG = nil
	result.clipboard, result.transferMove = snapshot, snapshot.Move
	if request.transfer.shortcuts {
		result.transferMove = false
		result.transferred, result.err = l.createShortcuts(snapshot.Paths, request.path)
		if result.transferred.Attempted {
			l.refreshTransferPanels(&result)
		}
		return result
	}
	sameFolder, err := browser.SourcesInDirectory(snapshot.Paths, request.path)
	if err != nil {
		result.err = err
		return result
	}
	if sameFolder && snapshot.Move {
		result.alreadyHere = true
		return result
	}
	duplicate := request.transfer.duplicate || sameFolder
	if duplicate && len(snapshot.Paths) == 1 {
		result.duplicatePlan, result.err = browser.PlanDuplicate(snapshot.Paths, request.path)
		return result
	}
	native := browser.TransferRequest{Paths: snapshot.Paths, Directory: request.path, Move: snapshot.Move, Duplicate: duplicate, Owner: request.transfer.owner, OpenFolders: result.locations}
	result.transferred, result.err = l.transfer(native)
	var moved []string
	for _, item := range result.transferred.Items {
		if item.Moved {
			moved = append(moved, item.Source)
		}
	}
	if len(moved) > 0 && request.transfer.snapshot == nil {
		result.clipboard, result.clipboardErr = l.completeClipboard(snapshot, moved)
	}
	if result.transferred.Attempted {
		l.refreshTransferPanels(&result)
	}
	return result
}

func (l *directoryLoader) archiveCopyOperation(request loadRequest, result loadResult) loadResult {
	selection := request.transfer.archive
	stage, err := browser.StageArchive(selection.location, selection.names)
	if err != nil {
		result.err = err
		return result
	}
	result.transferred, result.err = l.transfer(browser.TransferRequest{
		Paths: stage.Paths, Directory: request.path, Owner: request.transfer.owner, OpenFolders: result.locations,
	})
	// Remove temporary sources before reading open folders: the destination may
	// itself be the system temporary directory.
	stage.Close()
	if result.transferred.Attempted {
		l.refreshTransferPanels(&result)
	}
	return result
}
