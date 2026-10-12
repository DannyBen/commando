package app

import (
	"fmt"
	"path/filepath"

	"github.com/dannyben/commando/internal/feedback"
)

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
				other.applyTransferredEntry(item)
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
