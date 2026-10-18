package app

import (
	"fmt"

	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func (v *view) acceptFileOperation(result loadResult) {
	p := &v.panels[result.request.panel]
	p.loading = false
	if capture := result.request.clipboard; capture != nil {
		if result.err == nil {
			v.rememberCutClipboard(result.clipboard, capture.locations, result.locations)
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
	if result.duplicatePlan != nil || len(result.clipboard.ImagePNG) > 0 && len(result.clipboard.Paths) == 0 {
		v.transferPending = true
		v.pendingNameDraft = &result
		return
	}
	locations := result.request.transfer.locations
	refreshFailed := v.applyTransferPanels(result, locations)
	if result.request.transfer.snapshot == nil && !result.request.transfer.shortcuts {
		v.rememberCutClipboard(result.clipboard, locations, result.locations)
	}
	if result.request.transfer.shortcuts && len(result.transferred.Items) > 0 && samePath(p.path, result.request.path) {
		for row, item := range p.items {
			if item.file.Name == result.transferred.Items[0].Entry.Name {
				p.selected, p.reveal = row, true
				break
			}
		}
	}
	v.showTransferResult(result, refreshFailed)
	if message := p.feedback.Current(); message != nil {
		browser.TraceTransferFeedback(result.transferred, message.Text)
	}
}

func (v *view) applyTransferPanels(result loadResult, locations []string) (refreshFailed bool) {
	for pi := range v.panels {
		p := &v.panels[pi]
		if p.drives || pi >= len(locations) || !samePath(p.path, locations[pi]) || pi >= len(result.panelUpdates) {
			continue
		}
		update := result.panelUpdates[pi]
		if !update.needed {
			continue
		}
		if update.path != "" {
			p.path = update.path
		}
		if update.err != nil {
			refreshFailed = true
			v.showFeedback(pi, feedback.Warning, "Cannot refresh folder")
		} else {
			p.reconcileDirectoryListing(update.directory)
		}
	}
	return
}

func (v *view) showTransferResult(result loadResult, refreshFailed bool) {
	summary := result.transferred.Summary
	count := summary.Completed
	panel := result.request.panel
	shortcuts := result.request.transfer != nil && result.request.transfer.shortcuts
	verb := "transferred"
	if shortcuts {
		verb = "created"
	}
	if result.err != nil {
		message := fileTransferError(result.err)
		if shortcuts && message == "Cannot transfer" {
			message = "Cannot create shortcuts"
		}
		if count > 0 {
			message = fmt.Sprintf("%d %s; %s", count, verb, message)
		} else if summary.Partial > 0 {
			message = fmt.Sprintf("%d partial; %s", summary.Partial, message)
		}
		v.showFeedback(panel, feedback.Error, message)
	} else if result.clipboardErr != nil {
		v.showFeedback(panel, feedback.Warning, fmt.Sprintf("%d moved; clipboard update failed", count))
	} else if refreshFailed {
		v.showFeedback(panel, feedback.Warning, fmt.Sprintf("%d %s; refresh failed", count, verb))
	} else if result.transferred.Aborted {
		message := "Transfer stopped"
		if count > 0 {
			message = fmt.Sprintf("%d transferred; operation stopped", count)
		} else if summary.Partial > 0 {
			message = fmt.Sprintf("%d partial; operation stopped", summary.Partial)
		}
		v.showFeedback(panel, feedback.Info, message)
	} else if summary.Partial > 0 {
		v.showFeedback(panel, feedback.Warning, "Transfer partly completed")
	} else if summary.Failed > 0 || summary.Unfinished > 0 {
		message := "Transfer incomplete"
		if count > 0 {
			message = fmt.Sprintf("%d transferred; transfer incomplete", count)
		}
		v.showFeedback(panel, feedback.Warning, message)
	} else if summary.Skipped > 0 {
		v.showFeedback(panel, feedback.Warning, fmt.Sprintf("%d transferred; %d skipped", count, summary.Skipped))
	} else {
		verb := "copied"
		if result.transferMove {
			verb = "moved"
		} else if shortcuts {
			verb = "shortcuts created"
			if count == 1 {
				verb = "shortcut created"
			}
		}
		v.showFeedback(panel, feedback.Success, fmt.Sprintf("%d %s", count, verb))
	}
}
