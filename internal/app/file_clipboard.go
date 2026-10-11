package app

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

type fileClipboardRequest struct {
	paths     []string
	move      bool
	locations []string
}

type transferRequest struct {
	locations []string
	duplicate bool // Direct duplication does not read or change the clipboard.
	snapshot  *browser.FileClipboard
	plan      *browser.TransferPlan
	policy    browser.TransferPolicy
}

func (v *view) fileOperationLocations() []string {
	paths := make([]string, len(v.panels))
	for i := range v.panels {
		if !v.panels[i].drives {
			paths[i] = v.panels[i].path
		}
	}
	return paths
}

func (v *view) clipboardSelected(move bool) {
	p := &v.panels[v.active]
	if v.activeEditor() != nil || v.loader == nil || p.loading || p.drives || p.path == "" || v.transferPending {
		return
	}
	paths := p.selectedPaths()
	if len(paths) == 0 {
		return
	}
	p.loading, p.prefix = true, ""
	v.loader.requests <- loadRequest{panel: v.active, path: p.path, clipboard: &fileClipboardRequest{paths: paths, move: move, locations: v.fileOperationLocations()}}
}

func (v *view) pasteFiles() {
	p := &v.panels[v.active]
	if v.activeEditor() != nil || v.loader == nil || p.loading || p.drives || p.path == "" || v.transferPending || v.recyclingInProgress() {
		return
	}
	p.loading, p.prefix, v.transferPending = true, "", true
	// No transfer-progress UI until its interaction model has been agreed.
	v.loader.requests <- loadRequest{panel: v.active, path: p.path, transfer: &transferRequest{locations: v.fileOperationLocations()}}
}

func resolveFileLocation(path string) (string, error) {
	resolved, err := browser.ResolveOperationPath(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(resolved)
}

func (l *directoryLoader) fileOperation(request loadRequest) loadResult {
	result := loadResult{request: request}
	locations := []string(nil)
	if request.clipboard != nil {
		locations = request.clipboard.locations
	} else {
		locations = request.transfer.locations
	}
	result.locations = make([]string, len(locations))
	for pi, path := range locations {
		if path != "" {
			result.locations[pi], _ = resolveFileLocation(path)
		}
	}
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
		} else if plan.Merges {
			result.transferred, result.err = l.executeTransfer(plan, snapshot.Move, browser.TransferExclusive)
		} else {
			result.transferred, result.err = l.transfer(snapshot.Paths, request.path, snapshot.Move)
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

func (v *view) acceptFileOperation(result loadResult) {
	p := &v.panels[result.request.panel]
	p.loading = false
	locations := []string(nil)
	if capture := result.request.clipboard; capture != nil {
		locations = capture.locations
		if result.err == nil {
			v.rememberCutClipboard(result.clipboard, locations, result.locations)
			verb := "copied"
			if capture.move {
				verb = "cut"
			}
			v.showFeedback(result.request.panel, feedback.Info, fmt.Sprintf("%d %s to clipboard", len(result.clipboard.Paths), verb))
		} else {
			v.showFeedback(result.request.panel, feedback.Error, fileTransferError(result.err))
		}
		return
	}
	v.transferPending = false
	if result.alreadyHere {
		v.showFeedback(result.request.panel, feedback.Info, "Already in this folder")
		return
	}
	if result.duplicatePlan != nil {
		v.transferPending = true
		v.pendingDuplicate = &result
		return
	}
	if result.conflictPlan != nil {
		v.transferPending = true
		v.pendingFileConflict = &result
		v.offerFileConflict()
		return
	}
	locations = result.request.transfer.locations
	refreshFailed := false
	for pi := range v.panels {
		other := &v.panels[pi]
		if other.drives || pi >= len(locations) || !samePath(other.path, locations[pi]) || pi >= len(result.locations) {
			continue
		}
		if pi < len(result.panelUpdates) && result.panelUpdates[pi].needed {
			update := result.panelUpdates[pi]
			if update.path != "" {
				other.path = update.path
			}
			if update.err != nil {
				refreshFailed = true
				v.showFeedback(pi, feedback.Warning, "Cannot refresh folder")
			} else {
				other.reconcileTransferListing(update.directory)
			}
			continue
		}
		for index, item := range result.transferred.Items {
			if item.Moved && item.Entry.Directory && index < len(result.sourceParents) {
				source := filepath.Join(result.sourceParents[index], filepath.Base(item.Source))
				if relative, ok := movedLocationSuffix(source, result.locations[pi]); ok {
					other.path = filepath.Join(result.transferred.Directory, item.Entry.Name, relative)
				}
			}
			if item.Moved && index < len(result.sourceParents) && samePath(result.locations[pi], result.sourceParents[index]) {
				other.removeEntries([]string{filepath.Base(item.Source)})
			}
			if samePath(result.locations[pi], result.transferred.Directory) {
				first := other.list.Position.First
				inserted := other.applyTransferredEntry(item)
				// Preserve the visible item when insertion sorts above the viewport.
				if inserted && first > 0 {
					for row, entry := range other.items {
						if entry.file.Name == item.Entry.Name && row <= first {
							other.list.Position.First++
							break
						}
					}
				}
			}
		}
	}
	if !result.request.transfer.duplicate {
		v.rememberCutClipboard(result.clipboard, locations, result.locations)
	}
	verb := "copied"
	if result.transferMove {
		verb = "moved"
	}
	count := len(result.transferred.Items)
	if result.err != nil {
		message := fileTransferError(result.err)
		if count > 0 {
			message = fmt.Sprintf("%d transferred; %s", count, message)
		}
		v.showFeedback(result.request.panel, feedback.Error, message)
	} else if result.clipboardErr != nil {
		v.showFeedback(result.request.panel, feedback.Warning, fmt.Sprintf("%d moved; clipboard update failed", count))
	} else if refreshFailed {
		v.showFeedback(result.request.panel, feedback.Warning, fmt.Sprintf("%d transferred; refresh failed", count))
	} else if result.transferred.Skipped > 0 {
		v.showFeedback(result.request.panel, feedback.Warning, fmt.Sprintf("%d transferred; %d skipped", count, result.transferred.Skipped))
	} else {
		v.showFeedback(result.request.panel, feedback.Success, fmt.Sprintf("%d %s", count, verb))
	}
}

func movedLocationSuffix(source, path string) (string, bool) {
	if samePath(source, path) {
		return "", true
	}
	if len(path) > len(source) && samePath(source, path[:len(source)]) && path[len(source)] == filepath.Separator {
		return path[len(source)+1:], true
	}
	return "", false
}

func (v *view) rememberCutClipboard(snapshot browser.FileClipboard, paths, locations []string) {
	if snapshot.Move && len(snapshot.Paths) > 0 {
		v.cutClipboard, v.cutPanelPaths, v.cutLocations = snapshot, paths, locations
	} else {
		v.cutClipboard, v.cutPanelPaths, v.cutLocations = browser.FileClipboard{}, nil, nil
	}
}

func (v *view) updateCutClipboard(gtx layout.Context) {
	if len(v.cutClipboard.Paths) == 0 {
		return
	}
	if v.clipboardSequence() != v.cutClipboard.Sequence {
		v.rememberCutClipboard(browser.FileClipboard{}, nil, nil)
		return
	}
	gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(500 * time.Millisecond)})
}

func (v *view) cutEntry(panel, row int) bool {
	p := &v.panels[panel]
	if !p.markable(row) || !v.cutClipboard.Move {
		return false
	}
	path := p.path
	if panel < len(v.cutPanelPaths) && samePath(p.path, v.cutPanelPaths[panel]) && panel < len(v.cutLocations) && v.cutLocations[panel] != "" {
		path = v.cutLocations[panel]
	}
	path = filepath.Join(path, p.items[row].file.Name)
	for _, cut := range v.cutClipboard.Paths {
		if samePath(path, cut) {
			return true
		}
	}
	return false
}

func fileTransferError(err error) string {
	message := "Cannot transfer"
	switch {
	case errors.Is(err, browser.ErrNoClipboardFiles):
		return "No clipboard files"
	case errors.Is(err, browser.ErrInvalidName):
		return "Invalid filename"
	case errors.Is(err, fs.ErrExist):
		message = "Name already exists"
	case errors.Is(err, fs.ErrPermission):
		message = "Access denied"
	case errors.Is(err, fs.ErrNotExist):
		message = "Item not found"
	case errors.Is(err, browser.ErrTransferIntoSelf):
		message = "Destination inside source"
	case errors.Is(err, browser.ErrOverlappingSources):
		message = "Overlapping clipboard items"
	case errors.Is(err, browser.ErrSourceChanged):
		message = "Copied; source changed"
	case errors.Is(err, browser.ErrSameTransferFile):
		message = "Same file"
	case errors.Is(err, browser.ErrConflictChanged):
		return "Files changed; retry"
	case errors.Is(err, browser.ErrTransferType):
		return "File/folder type conflict"
	case errors.Is(err, browser.ErrDriveMapping), errors.Is(err, browser.ErrDriveMappingCycle), errors.Is(err, browser.ErrPathIdentity):
		return "Backing path unavailable"
	case errors.Is(err, errors.ErrUnsupported):
		message = "Unsupported file type"
	}
	return message
}
