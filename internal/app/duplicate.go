package app

import (
	"path/filepath"

	"gioui.org/layout"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

type duplicateRequest struct {
	plan      *browser.DuplicatePlan
	name      string
	locations []string
	owner     uintptr
}

func (p *panel) selectedPaths() []string {
	if !p.nativeFolder() {
		return nil
	}
	names := p.selectedNames()
	paths := make([]string, 0, len(names))
	for _, name := range names {
		paths = append(paths, filepath.Join(p.path, name))
	}
	return paths
}

func (p *panel) selectedNames() []string {
	var names []string
	if len(p.marks) > 0 {
		for row, item := range p.items {
			if p.marks[row] && p.markable(row) {
				names = append(names, item.file.Name)
			}
		}
	} else if p.markable(p.selected) {
		names = append(names, p.items[p.selected].file.Name)
	}
	return names
}

func (v *view) duplicateSelected() {
	p := &v.panels[v.active]
	if !v.interaction.idle() || v.loader == nil || p.loading || !p.nativeFolder() || p.path == "" || !v.actionAllowed(actionMutation) {
		return
	}
	paths := p.selectedPaths()
	if len(paths) == 0 {
		return
	}
	p.loading, p.prefix, v.transferPending = true, "", true
	snapshot := browser.FileClipboard{Paths: paths}
	if !v.loader.submit(loadRequest{panel: v.active, path: p.path, transfer: &transferRequest{
		locations: v.fileOperationLocations(), snapshot: &snapshot, duplicate: true, owner: v.ownerWindow}}) {
		p.loading, v.transferPending = false, false
	}
}

func (v *view) offerNameDraft(gtx layout.Context) {
	if v.pendingNameDraft == nil {
		return
	}
	result := v.pendingNameDraft
	panel := result.request.panel
	if panel != v.active {
		v.pendingNameDraft, v.transferPending = nil, false
		message := "Copy cancelled"
		if len(result.clipboard.ImagePNG) > 0 {
			message = "Paste cancelled"
		}
		if result.request.fileDraft != nil {
			message = "Save cancelled"
		}
		v.showFeedback(panel, feedback.Info, message)
		return
	}
	if !v.interaction.idle() {
		return
	}
	v.pendingNameDraft = nil
	p := &v.panels[panel]
	if draft := result.request.fileDraft; draft != nil {
		v.beginFileDraft(gtx, panel, draft.name, draft.data)
		return
	}
	if len(result.clipboard.ImagePNG) > 0 {
		v.transferPending = false
		v.beginFileDraft(gtx, panel, "file.png", result.clipboard.ImagePNG)
		return
	}
	source, name := result.duplicatePlan.DuplicateSubject()
	// Anchor to the copied source even when a different row is highlighted.
	p.applyTransferredEntry(browser.TransferredItem{Entry: source, Replaced: true})
	row := p.selected + 1
	for index, item := range p.items {
		if samePath(item.file.Name, source.Name) {
			row = index + 1
			break
		}
	}
	r := &nameSession{kind: nameDuplicate, panel: panel, row: row, path: p.path, folder: source.Directory,
		duplicate: result.duplicatePlan, previousPosition: p.list.Position, previousReveal: p.reveal}
	v.beginName(gtx, r, name, source.Directory)
}

func (l *directoryLoader) duplicateOperation(request loadRequest) loadResult {
	r := request.duplicate
	result := loadResult{request: request, locations: resolveFileLocations(r.locations)}
	nativeRequest, err := r.plan.Request(r.name, r.owner)
	if err != nil {
		result.err = err
		return result
	}
	nativeRequest.OpenFolders = result.locations
	result.transferred, result.err = l.transfer(nativeRequest)
	if result.transferred.Attempted {
		l.refreshTransferPanels(&result)
	}
	if len(result.transferred.Items) > 0 {
		result.created = result.transferred.Items[0].Entry
	}
	return result
}

func (v *view) applyDuplicate(result loadResult) {
	p := &v.panels[result.request.panel]
	for row, item := range p.items {
		if samePath(item.file.Name, result.created.Name) {
			p.selected, p.reveal = row, true
			break
		}
	}
}
