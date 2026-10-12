package app

import (
	"errors"
	"fmt"

	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func (v *view) offerFileConflict() {
	if v.pendingFileConflict == nil {
		return
	}
	if panel := v.pendingFileConflict.request.panel; panel != v.active {
		v.pendingFileConflict = nil
		v.transferPending = false
		v.showFeedback(panel, feedback.Info, "Transfer cancelled")
		return
	}
	if v.activeEditor() != nil || v.actions != nil {
		return
	}
	result := *v.pendingFileConflict
	v.pendingFileConflict = nil
	panel := result.request.panel
	p := &v.panels[panel]
	plan := result.conflictPlan
	file := plan.Subject
	p.applyTransferredEntry(browser.TransferredItem{Entry: file, Replaced: true})
	row := 0
	for index, item := range p.items {
		if samePath(item.file.Name, file.Name) {
			row = index + 1
			break
		}
	}
	if errors.Is(result.err, browser.ErrConflictChanged) {
		v.showFeedback(panel, feedback.Error, "Files changed; choose again")
	} else {
		message := "File exists"
		if !plan.SingleFile {
			message = fmt.Sprintf("%d files exist", plan.Conflicts)
			if plan.Conflicts == 1 {
				message = "1 file exists"
			}
		}
		v.showFeedback(panel, feedback.Warning, message)
	}
	options := []string{"Overwrite", "Cancel"}
	if !plan.SingleFile {
		options = []string{"Overwrite all", "Skip existing", "Cancel"}
	}
	v.startInlineActions(panel, row, "", "", options, func(choice int) {
		if choice < len(options)-1 {
			snapshot := result.clipboard
			policy := browser.TransferOverwrite
			if choice == 1 {
				policy = browser.TransferSkipExisting
			}
			v.panels[panel].loading = true
			v.loader.requests <- loadRequest{panel: panel, path: result.request.path, transfer: &transferRequest{
				locations: v.fileOperationLocations(), snapshot: &snapshot, plan: plan, policy: policy}}
		} else {
			v.transferPending = false
			v.showFeedback(panel, feedback.Info, "Transfer cancelled")
		}
	}, func() { v.transferPending = false; v.showFeedback(panel, feedback.Info, "Transfer cancelled") })
	v.actions.subjectRow = row - 1
}
