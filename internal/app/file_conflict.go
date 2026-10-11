package app

import (
	"errors"

	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func (v *view) offerFileConflict() {
	if v.pendingFileConflict == nil || v.activeEditor() != nil || v.actions != nil {
		return
	}
	result := *v.pendingFileConflict
	v.pendingFileConflict = nil
	panel := result.request.panel
	p := &v.panels[panel]
	file := result.fileConflict.DestinationEntry()
	first := p.list.Position.First
	inserted := p.applyTransferredEntry(browser.TransferredItem{Entry: file, Replaced: true})
	row := 0
	for index, item := range p.items {
		if samePath(item.file.Name, file.Name) {
			row = index + 1
			if inserted && first > 0 && index <= first {
				p.list.Position.First++
			}
			break
		}
	}
	if errors.Is(result.err, browser.ErrConflictChanged) {
		v.showFeedback(panel, feedback.Error, "Files changed; choose again")
	}
	v.startInlineActions(panel, row, "", "", []string{"Overwrite", "Cancel"}, func(choice int) {
		switch choice {
		case 0:
			snapshot := result.clipboard
			v.panels[panel].loading = true
			v.loader.requests <- loadRequest{panel: panel, path: result.request.path, transfer: &transferRequest{
				locations: v.fileOperationLocations(), snapshot: &snapshot, overwrite: result.fileConflict}}
		case 1:
			v.transferPending = false
			v.showFeedback(panel, feedback.Info, "Transfer cancelled")
		}
	}, func() { v.transferPending = false; v.showFeedback(panel, feedback.Info, "Transfer cancelled") })
	v.actions.subjectRow = row - 1
}
