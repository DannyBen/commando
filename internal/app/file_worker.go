package app

import (
	"errors"
	"io/fs"
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
	var snapshot browser.FileClipboard
	var err error
	if request.transfer.snapshot != nil {
		snapshot = *request.transfer.snapshot
	} else {
		snapshot, err = l.readClipboard()
	}
	if err != nil {
		result.err = err
		return result
	}
	if len(snapshot.Paths) == 0 {
		result.err = browser.ErrNoClipboardFiles
		return result
	}
	result.clipboard = snapshot
	result.transferMove = snapshot.Move
	if request.transfer.plan == nil {
		sameFolder, checkErr := browser.SourcesInDirectory(snapshot.Paths, request.path)
		if checkErr != nil {
			result.err = checkErr
			return result
		}
		if sameFolder && snapshot.Move {
			result.alreadyHere = true
			return result
		}
		if request.transfer.duplicate || sameFolder {
			plan, planErr := browser.PlanDuplicate(snapshot.Paths, request.path)
			if planErr != nil {
				result.err = planErr
			} else if len(snapshot.Paths) == 1 {
				result.duplicatePlan = plan
			} else {
				result.transferred, result.err = l.executeTransfer(plan, false, browser.TransferExclusive)
			}
			return result
		}
	}
	if plan := request.transfer.plan; plan != nil {
		result.transferred, result.err = l.executeTransfer(plan, snapshot.Move, request.transfer.policy)
	} else {
		plan, planErr := l.planTransfer(snapshot.Paths, request.path)
		if planErr != nil {
			result.err = planErr
		} else if plan.Conflicts > 0 {
			result.conflictPlan = plan
		} else {
			result.transferred, result.err = l.executeTransfer(plan, snapshot.Move, browser.TransferExclusive)
		}
	}
	if len(result.transferred.Items) == 0 && (errors.Is(result.err, fs.ErrExist) || errors.Is(result.err, browser.ErrConflictChanged)) {
		plan, inspectErr := l.planTransfer(snapshot.Paths, request.path)
		if inspectErr == nil && plan.Conflicts > 0 {
			result.conflictPlan = plan
		} else if inspectErr != nil {
			result.err = inspectErr
		}
	}
	var moved []string
	for _, item := range result.transferred.Items {
		parent, _ := resolveFileLocation(filepath.Dir(item.Source))
		result.sourceParents = append(result.sourceParents, parent)
		if item.Moved {
			moved = append(moved, item.Source)
		}
	}
	if len(moved) > 0 {
		result.clipboard, result.clipboardErr = l.completeClipboard(snapshot, moved)
	}
	if len(result.transferred.Changes) > 0 {
		l.refreshTransferPanels(&result)
	}
	return result
}
